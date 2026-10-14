package devicelog

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func ligado(t *testing.T) (*Logger, Options) {
	t.Helper()
	o := optsDeTeste(t)
	l := New(o, infoDeTeste(), nil)
	if err := l.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	return l, o
}

func routerDeTeste(l *Logger, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(l.Gin())
	r.Any("/*rota", handler)
	return r
}

func TestGinRegistraRequisicaoComCorpoECampos(t *testing.T) {
	l, o := ligado(t)
	var recebido string
	r := routerDeTeste(l, func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		recebido = string(b)
		c.JSON(200, gin.H{"statusCode": 1})
	})

	corpo := `{"UserInfo":{"employeeNo":"123","name":"Ana"}}`
	req := httptest.NewRequest("PUT", "/ISAPI/AccessControl/UserInfo/Modify?format=json", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", `Digest username="admin", realm="x"`)
	req.RemoteAddr = "10.0.0.5:51234"
	r.ServeHTTP(httptest.NewRecorder(), req)

	if recebido != corpo {
		t.Fatalf("handler recebeu %q", recebido)
	}
	got := ler(t, o.FilePath(1, 0))
	for _, quero := range []string{
		"IN 10.0.0.5:51234 user=admin PUT /ISAPI/AccessControl/UserInfo/Modify?format=json -> 200 ",
		"| employeeNo=123 name=Ana",
		"\n    > " + corpo,
		"\n    < {\"statusCode\":1}",
	} {
		if !strings.Contains(got, quero) {
			t.Errorf("falta %q em:\n%s", quero, got)
		}
	}
	if strings.Contains(got, "Digest") {
		t.Errorf("header Authorization vazou:\n%s", got)
	}
}

func TestGinCorpoGrandeChegaInteiroAoHandler(t *testing.T) {
	l, o := ligado(t)
	grande := bytes.Repeat([]byte("a"), maxCopy+10)
	var n int
	r := routerDeTeste(l, func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		n = len(b)
		c.Status(200)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/face", bytes.NewReader(grande)))

	if n != len(grande) {
		t.Fatalf("handler recebeu %d bytes, quero %d", n, len(grande))
	}
	got := ler(t, o.FilePath(1, 0))
	if !strings.Contains(got, "> <corpo 2.0MB não copiado>") {
		t.Errorf("quero só o tamanho:\n%s", got)
	}
	if strings.Contains(got, "aaaa") {
		t.Errorf("corpo grande foi gravado")
	}
}

func TestGinDesligadoNaoLeNemGrava(t *testing.T) {
	o := optsDeTeste(t)
	l := New(o, infoDeTeste(), nil)
	r := routerDeTeste(l, func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		c.String(200, string(b))
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/x", strings.NewReader("abc")))
	if rec.Body.String() != "abc" {
		t.Fatalf("resposta %q", rec.Body.String())
	}
	if fs := o.Files(1); len(fs) != 0 {
		t.Errorf("gravou desligado: %v", fs)
	}
}

func TestGinStreamMarcadoNaoGeraLinhaIN(t *testing.T) {
	l, o := ligado(t)
	r := routerDeTeste(l, func(c *gin.Context) {
		MarkStream(c)
		c.Status(200)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/alertStream", nil))
	if strings.Contains(ler(t, o.FilePath(1, 0)), " IN ") {
		t.Errorf("stream não pode gerar linha IN")
	}
}

type connFalsa struct {
	net.Conn
	addr string
}

func (c connFalsa) RemoteAddr() net.Addr {
	a, _ := net.ResolveTCPAddr("tcp", c.addr)
	return a
}

func TestConnStateAbreContaEFecha(t *testing.T) {
	l, o := ligado(t)
	c := connFalsa{addr: "10.0.0.5:51234"}
	l.ConnState(c, http.StateNew)
	l.ConnState(c, http.StateActive)
	l.ConnState(c, http.StateIdle)
	l.ConnState(c, http.StateActive)
	l.ConnState(c, http.StateClosed)

	got := ler(t, o.FilePath(1, 0))
	if !strings.Contains(got, "CONN 10.0.0.5:51234 aberta") {
		t.Errorf("sem aberta:\n%s", got)
	}
	if !strings.Contains(got, "CONN 10.0.0.5:51234 fechada (2 reqs, 0s)") {
		t.Errorf("sem fechada:\n%s", got)
	}
}

func TestConnStateHijackedViraStream(t *testing.T) {
	l, o := ligado(t)
	c := connFalsa{addr: "10.0.0.6:6000"}
	l.ConnState(c, http.StateNew)
	l.ConnState(c, http.StateActive)
	l.ConnState(c, http.StateHijacked)
	if !strings.Contains(ler(t, o.FilePath(1, 0)), "CONN 10.0.0.6:6000 virou stream (1 reqs, 0s)") {
		t.Errorf("sem virou stream")
	}
}

func TestStreamAbreEventoFecha(t *testing.T) {
	l, o := ligado(t)
	l.StreamOpen("10.0.0.5:6001", "/ISAPI/Event/notification/alertStream")
	l.StreamEvent("10.0.0.5:6001", "cardNo=999", 2048)
	l.StreamEvent("10.0.0.5:6001", "", 100)
	l.StreamClose("10.0.0.5:6001")

	got := ler(t, o.FilePath(1, 0))
	for _, quero := range []string{
		"CONECTADO 10.0.0.5:6001 /ISAPI/Event/notification/alertStream",
		"OUT -> 10.0.0.5:6001 stream 2KB | cardNo=999",
		"OUT -> 10.0.0.5:6001 stream 100B\n",
		"DESCONECTADO 10.0.0.5:6001 /ISAPI/Event/notification/alertStream após 0s, 2 eventos",
	} {
		if !strings.Contains(got, quero) {
			t.Errorf("falta %q em:\n%s", quero, got)
		}
	}
}

func TestDesligarComStreamAbertoNaoGrava(t *testing.T) {
	l, o := ligado(t)
	l.StreamOpen("10.0.0.5:6001", "/alertStream")
	if err := l.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	l.StreamEvent("10.0.0.5:6001", "x", 10)
	l.StreamClose("10.0.0.5:6001")
	if fs := o.Files(1); len(fs) != 0 {
		t.Errorf("gravou depois de desligar: %v", fs)
	}
}

func TestOutRegistraSucessoEErro(t *testing.T) {
	l, o := ligado(t)
	l.Out("POST", "http://10.0.0.5:8000/w-access", "application/json",
		[]byte(`{"cardNo":"999"}`), 200, nil, 30*time.Millisecond)
	l.Out("GET", "http://10.0.0.5:8000/keepalive", "", nil, 0,
		errors.New("connection refused"), 2*time.Millisecond)

	got := ler(t, o.FilePath(1, 0))
	for _, quero := range []string{
		"OUT -> POST http://10.0.0.5:8000/w-access -> 200 30ms | cardNo=999\n    > {\"cardNo\":\"999\"}",
		"OUT -> GET http://10.0.0.5:8000/keepalive -> erro: connection refused 2ms\n",
	} {
		if !strings.Contains(got, quero) {
			t.Errorf("falta %q em:\n%s", quero, got)
		}
	}
}
