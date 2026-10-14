package devicelog

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// maxCapture é quanto da resposta guardamos para descrever: o bastante para
// a sanitização reconhecer um base64 longo antes de cortar em maxBody.
const maxCapture = 64 << 10

const streamKey = "devicelog.stream"

type connInfo struct {
	opened time.Time
	reqs   int
}

type streamInfo struct {
	opened time.Time
	path   string
	events int
}

// MarkStream avisa o middleware que o handler assumiu a conexão como
// stream. O próprio handler registra CONECTADO/DESCONECTADO.
func MarkStream(c *gin.Context) {
	c.Set(streamKey, true)
}

type bodyCapture struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *bodyCapture) keep(b []byte) {
	if room := maxCapture - w.buf.Len(); room > 0 {
		if len(b) > room {
			b = b[:room]
		}
		w.buf.Write(b)
	}
}

func (w *bodyCapture) Write(b []byte) (int, error) {
	w.keep(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyCapture) WriteString(s string) (int, error) {
	w.keep([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

// restoredBody devolve ao handler o que o middleware já leu, seguido do
// resto do corpo original.
type restoredBody struct {
	io.Reader
	io.Closer
}

// Gin registra cada requisição recebida. Desligado, não lê nada.
func (l *Logger) Gin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Enabled() {
			c.Next()
			return
		}
		inicio := time.Now()
		req := c.Request

		var reqBody []byte
		lidos, grande := 0, false
		if req.Body != nil && req.Body != http.NoBody {
			data, _ := io.ReadAll(io.LimitReader(req.Body, maxCopy+1))
			lidos, grande = len(data), len(data) > maxCopy
			req.Body = restoredBody{io.MultiReader(bytes.NewReader(data), req.Body), req.Body}
			if !grande {
				reqBody = data
			}
		}

		w := &bodyCapture{ResponseWriter: c.Writer}
		c.Writer = w
		c.Next()

		if c.GetBool(streamKey) {
			return
		}

		respSize := w.Size()
		if respSize < 0 {
			respSize = 0
		}
		tamReq := humanSize(lidos)
		if grande {
			tamReq = ">" + humanSize(maxCopy)
		}

		var b strings.Builder
		b.WriteString(req.RemoteAddr)
		if u := UserFromAuth(req.Header.Get("Authorization")); u != "" {
			b.WriteString(" user=" + u)
		}
		fmt.Fprintf(&b, " %s %s -> %d %dms (%s/%s)", req.Method, maskURI(req.URL.RequestURI()),
			w.Status(), time.Since(inicio).Milliseconds(), tamReq, humanSize(respSize))
		ct := req.Header.Get("Content-Type")
		if f := KeyFields(ct, reqBody, req.URL.Query()); f != "" {
			b.WriteString(" | " + f)
		}
		switch {
		case grande:
			fmt.Fprintf(&b, "\n    > <corpo %s não copiado>", humanSize(maxCopy))
		case len(reqBody) > 0:
			if d := Describe(ct, reqBody); d != "" {
				b.WriteString("\n    > " + d)
			}
		}
		if d := Describe(w.Header().Get("Content-Type"), w.buf.Bytes()); d != "" {
			b.WriteString("\n    < " + d)
		}
		l.Printf("IN", "%s", b.String())
	}
}

func (ci *connInfo) resumo(now time.Time) string {
	if ci == nil {
		return "? reqs"
	}
	s := fmt.Sprintf("%d reqs", ci.reqs)
	if !ci.opened.IsZero() {
		s += ", " + now.Sub(ci.opened).Round(time.Second).String()
	}
	return s
}

// ConnState é o hook de http.Server: uma linha quando a conexão TCP abre e
// outra quando fecha. Dois gerenciadores no mesmo emulador aparecem aqui
// como dois endereços alternando.
func (l *Logger) ConnState(conn net.Conn, state http.ConnState) {
	if !l.Enabled() {
		return
	}
	addr := conn.RemoteAddr().String()
	l.mu.Lock()
	defer l.mu.Unlock()

	ci := l.conns[addr]
	switch state {
	case http.StateNew:
		l.conns[addr] = &connInfo{opened: l.now()}
		l.writeLocked("CONN", addr+" aberta")
	case http.StateActive:
		if ci == nil {
			ci = &connInfo{}
			l.conns[addr] = ci
		}
		ci.reqs++
	case http.StateHijacked:
		delete(l.conns, addr)
		l.writeLocked("CONN", fmt.Sprintf("%s virou stream (%s)", addr, ci.resumo(l.now())))
	case http.StateClosed:
		delete(l.conns, addr)
		l.writeLocked("CONN", fmt.Sprintf("%s fechada (%s)", addr, ci.resumo(l.now())))
	}
}

// StreamOpen marca a abertura de um stream de eventos (alertStream,
// eventManager, snapManager).
func (l *Logger) StreamOpen(remote, path string) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.streams[remote] = &streamInfo{opened: l.now(), path: path}
	l.writeLocked("CONECTADO", remote+" "+path)
}

// StreamEvent registra um evento escrito no stream (heartbeat não entra).
func (l *Logger) StreamEvent(remote, summary string, size int) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if s := l.streams[remote]; s != nil {
		s.events++
	}
	msg := fmt.Sprintf("-> %s stream %s", remote, humanSize(size))
	if summary != "" {
		msg += " | " + summary
	}
	l.writeLocked("OUT", msg)
}

// StreamClose fecha o stream com duração e total de eventos.
func (l *Logger) StreamClose(remote string) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.streams[remote]
	delete(l.streams, remote)
	if s == nil {
		l.writeLocked("DESCONECTADO", remote)
		return
	}
	l.writeLocked("DESCONECTADO", fmt.Sprintf("%s %s após %s, %d eventos",
		remote, s.path, l.now().Sub(s.opened).Round(time.Second), s.events))
}

// Out registra um envio do emulador ao gerenciador (push de evento,
// keepalive).
func (l *Logger) Out(method, url, contentType string, body []byte, status int, err error, d time.Duration) {
	if !l.Enabled() {
		return
	}
	res := fmt.Sprint(status)
	if err != nil {
		res = "erro: " + err.Error()
	}
	msg := fmt.Sprintf("-> %s %s -> %s %dms", method, url, res, d.Milliseconds())
	if f := KeyFields(contentType, body, nil); f != "" {
		msg += " | " + f
	}
	if desc := Describe(contentType, body); desc != "" {
		msg += "\n    > " + desc
	}
	l.Printf("OUT", "%s", msg)
}
