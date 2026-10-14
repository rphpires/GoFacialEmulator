package hikvision

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"GoFacialEmulator/internal/devicelog"
)

func logDeTeste(t *testing.T) (*devicelog.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	l := devicelog.New(devicelog.Options{Dir: dir, MaxSize: 1 << 20, MaxFiles: 8},
		devicelog.Info{ID: 7, Name: "hik", Type: "Hikvision", Port: 4007}, nil)
	if err := l.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	return l, filepath.Join(dir, "device-7.log")
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

func TestHikvisionLogRegistraConexaoERequisicao(t *testing.T) {
	e := newTestEmulator(t)
	dlog, caminho := logDeTeste(t)
	e.dlog = dlog

	srv := httptest.NewUnstartedServer(e.newRouter())
	srv.Config.ConnState = e.dlog.ConnState
	srv.Start()

	resp, err := http.Get(srv.URL + "/rota-inexistente?employeeNo=55")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	srv.Close()

	esperarNoLog(t, caminho,
		" aberta",
		"IN 127.0.0.1:",
		"GET /rota-inexistente?employeeNo=55 -> 404 ",
		"| employeeNo=55",
		" fechada (1 reqs, ")
}

func TestHikvisionPostEventRegistraSaida(t *testing.T) {
	alvo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer alvo.Close()

	e := newTestEmulator(t)
	dlog, caminho := logDeTeste(t)
	e.dlog = dlog

	evento := &Event{EventType: "AccessControllerEvent"}
	evento.AccessControllerEvent.CardNo = "999"
	corpo := buildPushEventMultipart(evento, []byte{0xff, 0xd8, 0xff, 0x00}, "MIME_boundary")

	if err := e.postEvent(alvo.URL+"/w-access", "multipart/form-data; boundary=MIME_boundary", corpo, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	esperarNoLog(t, caminho,
		"OUT -> POST "+alvo.URL+"/w-access -> 200 ",
		"cardNo=999",
		"[Picture] <image/jpeg 4B>")
}

func TestHikvisionSemLoggerNaoQuebra(t *testing.T) {
	e := newTestEmulator(t) // dlog nil
	srv := httptest.NewServer(e.newRouter())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/rota-inexistente")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d", resp.StatusCode)
	}
}
