package dahua

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"GoFacialEmulator/internal/devicelog"
	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
)

func logDeTeste(t *testing.T) (*devicelog.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	l := devicelog.New(devicelog.Options{Dir: dir, MaxSize: 1 << 20, MaxFiles: 8},
		devicelog.Info{ID: 9, Name: "dahua", Type: "Dahua", Port: 4009}, nil)
	if err := l.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	return l, filepath.Join(dir, "device-9.log")
}

// esperarNoLog espera (até 2s) todos os trechos aparecerem: o fechamento da
// conexão chega ao hook ConnState de forma assíncrona.
func esperarNoLog(t *testing.T, caminho string, trechos ...string) {
	t.Helper()
	limite := time.Now().Add(2 * time.Second)
	for {
		b, _ := os.ReadFile(caminho)
		faltando := ""
		for _, tr := range trechos {
			if !strings.Contains(string(b), tr) {
				faltando = tr
				break
			}
		}
		if faltando == "" {
			return
		}
		if time.Now().After(limite) {
			t.Fatalf("falta %q no log:\n%s", faltando, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func emuladorDahuaDeTeste(dlog *devicelog.Logger) *Emulator {
	return &Emulator{
		tracer:   trace.NewTracer(),
		stopChan: make(chan struct{}),
		configs:  newConfigStore("aa:bb:cc:dd:ee:ff"),
		eventLog: newDahuaEventLog(10),
		pushChan: make(chan []byte, 8),
		dlog:     dlog,
	}
}

func TestDahuaLogRegistraConexaoERequisicao(t *testing.T) {
	dlog, caminho := logDeTeste(t)
	e := emuladorDahuaDeTeste(dlog)

	srv := httptest.NewUnstartedServer(e.newRouter())
	srv.Config.ConnState = e.dlog.ConnState
	srv.Start()

	resp, err := http.Get(srv.URL + "/cgi-bin/inexistente.cgi?action=insert&name=AccessUserCard")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	srv.Close()

	esperarNoLog(t, caminho,
		" aberta",
		"IN 127.0.0.1:",
		"GET /cgi-bin/inexistente.cgi?action=insert&name=AccessUserCard -> 404 ",
		"| action=insert name=AccessUserCard",
		" fechada (1 reqs, ")
}

func TestDahuaPostNotificationRegistraSaida(t *testing.T) {
	alvo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer alvo.Close()

	dlog, caminho := logDeTeste(t)
	e := emuladorDahuaDeTeste(dlog)

	corpo := "--myboundary\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n" +
		`{"Events":[{"Code":"DoorStatus","Data":{"Status":"Open"}}]}` + "\r\n--myboundary--\r\n"
	if err := e.postNotification(alvo.URL+"/notification", []byte(corpo), 2*time.Second); err != nil {
		t.Fatal(err)
	}
	esperarNoLog(t, caminho,
		"OUT -> POST "+alvo.URL+"/notification -> 200 ",
		"Code=DoorStatus Status=Open")
}

// Um handler que entra em pânico vira 500 e precisa aparecer no log: é o
// erro que o operador mais quer ver.
func TestDahuaLogRegistraRequisicaoQueEntrouEmPanico(t *testing.T) {
	dlog, caminho := logDeTeste(t)
	e := emuladorDahuaDeTeste(dlog)

	r := e.newRouter()
	r.GET("/panico", func(c *gin.Context) { panic("teste") })
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/panico")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	esperarNoLog(t, caminho, "GET /panico -> 500 ")
}
