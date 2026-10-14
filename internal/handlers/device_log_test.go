package handlers

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"GoFacialEmulator/internal/devicelog"

	"github.com/gin-gonic/gin"
)

// routerDeLog registra as rotas como em produção, junto de uma irmã com
// parâmetro, para pegar conflito de rota no gin.
func routerDeLog(t *testing.T) (*gin.Engine, *devicelog.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	reg := devicelog.NewRegistry(devicelog.Options{Dir: t.TempDir(), MaxSize: 1 << 20, MaxFiles: 8}, nil)
	h := &Handler{tracer: tracerDeTeste(t), logs: reg}
	r := gin.New()
	g := r.Group("/api/devices")
	g.GET("/:id/settings", func(c *gin.Context) { c.Status(204) })
	g.GET("/:id/log", h.getDeviceLog)
	g.GET("/:id/log.zip", h.getDeviceLogZip)
	return r, reg
}

func get(r *gin.Engine, alvo string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, alvo, nil))
	return rec
}

func TestGetDeviceLogServeArquivoAtivoInline(t *testing.T) {
	r, reg := routerDeLog(t)
	l := reg.For(devicelog.Info{ID: 3, Name: "x", Type: "Dahua", Port: 4003})
	_ = l.SetEnabled(true)
	l.Printf("IN", "linha de teste")

	rec := get(r, "/api/devices/3/log")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") {
		t.Errorf("Content-Disposition %q", cd)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("sem no-store")
	}
	if !strings.Contains(rec.Body.String(), "IN linha de teste") {
		t.Errorf("corpo: %s", rec.Body.String())
	}
	if get(r, "/api/devices/3/settings").Code != 204 {
		t.Error("rota irmã quebrou")
	}
}

func TestGetDeviceLogZip(t *testing.T) {
	r, reg := routerDeLog(t)
	_ = reg.SetEnabled(devicelog.Info{ID: 3}, true)

	rec := get(r, "/api/devices/3/log.zip")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="device-3-logs-`) {
		t.Errorf("Content-Disposition %q", cd)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "device-3.log" {
		t.Errorf("zip: %+v", zr.File)
	}
}

func TestGetDeviceLogSemArquivo404(t *testing.T) {
	r, _ := routerDeLog(t)
	for _, alvo := range []string{"/api/devices/9/log", "/api/devices/9/log.zip"} {
		if rec := get(r, alvo); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, quero 404", alvo, rec.Code)
		}
	}
}

func TestGetDeviceLogIDInvalido400(t *testing.T) {
	r, _ := routerDeLog(t)
	for _, alvo := range []string{"/api/devices/abc/log", "/api/devices/0/log.zip", "/api/devices/-1/log"} {
		if rec := get(r, alvo); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, quero 400", alvo, rec.Code)
		}
	}
}
