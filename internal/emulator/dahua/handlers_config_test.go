package dahua

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
)

func newConfigTestEmulator() *Emulator {
	return &Emulator{
		tracer:   trace.NewTracer(),
		stopChan: make(chan struct{}),
		configs:  newConfigStore("00:11:22:33:44:55"),
	}
}

func TestHandleConfigManager_GetAccessControl(t *testing.T) {
	e := newConfigTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/configManager.cgi", e.handleConfigManager)

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/configManager.cgi?action=getConfig&name=AccessControl", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "table.AccessControl[0].CardStoreFormat=0") {
		t.Errorf("CardStoreFormat ausente:\n%s", w.Body.String())
	}
}

func TestHandleConfigManager_SetThenGet(t *testing.T) {
	e := newConfigTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/configManager.cgi", e.handleConfigManager)

	set := httptest.NewRequest(http.MethodGet,
		"/cgi-bin/configManager.cgi?action=setConfig&AccessControl%5B0%5D.CardStoreFormat=1", nil)
	wSet := httptest.NewRecorder()
	r.ServeHTTP(wSet, set)
	if wSet.Code != http.StatusOK {
		t.Fatalf("setConfig status: got %d, want 200", wSet.Code)
	}

	get := httptest.NewRequest(http.MethodGet, "/cgi-bin/configManager.cgi?action=getConfig&name=AccessControl", nil)
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, get)
	if !strings.Contains(wGet.Body.String(), "table.AccessControl[0].CardStoreFormat=1") {
		t.Errorf("setConfig nao persistiu:\n%s", wGet.Body.String())
	}
}

func TestHandleConfigManager_GetNetworkStillWorks(t *testing.T) {
	e := newConfigTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/configManager.cgi", e.handleConfigManager)

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/configManager.cgi?action=getConfig&name=Network", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "table.Network.eth0.PhysicalAddress=00:11:22:33:44:55") {
		t.Errorf("__get_mac_address vai quebrar:\n%s", body)
	}
	if !strings.Contains(body, "\r\ntable.Network.eth0.SubnetMask=") {
		t.Error("o delimitador que __get_mac_address usa sumiu")
	}
}

func TestHandleConfigManager_GetUnknownTableIsBadRequest(t *testing.T) {
	e := newConfigTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/configManager.cgi", e.handleConfigManager)

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/configManager.cgi?action=getConfig&name=Blah", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestHandleConfigManager_GetAccessTimeSchedule(t *testing.T) {
	e := newConfigTestEmulator()
	r := gin.New()
	r.GET("/cgi-bin/configManager.cgi", e.handleConfigManager)

	req := httptest.NewRequest(http.MethodGet, "/cgi-bin/configManager.cgi?action=getConfig&name=AccessTimeSchedule", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestKeepAliveConfig_ReadsFromConfigStore(t *testing.T) {
	e := newConfigTestEmulator()

	if ativo, _, _ := e.keepAliveConfig(); ativo {
		t.Error("keepalive deve comecar desligado")
	}

	e.configs.Set("Intelbras_ModeCfg.KeepAlive.Enable", "true")
	e.configs.Set("Intelbras_ModeCfg.KeepAlive.Interval", "5")
	e.configs.Set("Intelbras_ModeCfg.KeepAlive.Path", "/keepalive")

	ativo, intervalo, path := e.keepAliveConfig()
	if !ativo {
		t.Fatal("keepalive deveria estar ligado")
	}
	if intervalo != 5*time.Second {
		t.Errorf("intervalo: got %v, want 5s", intervalo)
	}
	if path != "/keepalive" {
		t.Errorf("path: got %q, want /keepalive", path)
	}
}

func TestKeepAliveConfig_DefaultsWhenIncomplete(t *testing.T) {
	e := newConfigTestEmulator()
	e.configs.Set("Intelbras_ModeCfg.KeepAlive.Enable", "true")
	e.configs.Set("Intelbras_ModeCfg.KeepAlive.Interval", "")
	e.configs.Set("Intelbras_ModeCfg.KeepAlive.Path", "")

	_, intervalo, path := e.keepAliveConfig()
	if intervalo != 5*time.Second {
		t.Errorf("intervalo default: got %v, want 5s", intervalo)
	}
	if path != "/keepalive" {
		t.Errorf("path default: got %q, want /keepalive", path)
	}
}

func TestKeepAlive_CallsConfiguredPath(t *testing.T) {
	var batidas int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/keepalive" && r.Method == http.MethodGet {
			atomic.AddInt32(&batidas, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := newConfigTestEmulator()
	e.remoteServerURL = srv.URL
	e.configs.Set("Intelbras_ModeCfg.KeepAlive.Enable", "true")
	e.configs.Set("Intelbras_ModeCfg.KeepAlive.Interval", "1")

	e.startKeepAlive()
	defer close(e.stopChan)

	prazo := time.Now().Add(5 * time.Second)
	for time.Now().Before(prazo) {
		if atomic.LoadInt32(&batidas) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("o emulador nunca chamou GET /keepalive")
}
