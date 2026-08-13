package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"GoFacialEmulator/internal/monitoring"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// checkerFixo é um monitoring.HealthChecker com resultado fixo, pra montar
// cenários de teste (degraded/unhealthy) sem depender de checkers reais ou
// de um Handler completo — quickHealthCheck e advancedHealthCheck só leem
// h.healthMonitor, então um *Handler{healthMonitor: hm} basta.
type checkerFixo struct {
	nome   string
	status monitoring.HealthStatus
}

func (c *checkerFixo) Name() string { return c.nome }
func (c *checkerFixo) Check(ctx context.Context) monitoring.ComponentHealth {
	return monitoring.ComponentHealth{Name: c.nome, Status: c.status}
}

// TestQuickHealthCheck_ContratoHTTP e TestAdvancedHealthCheck_ContratoHTTP
// testam o contrato HTTP de verdade (via httptest), não só o enum de
// status: é exatamente o que INICIAR.bat/iniciar.sh dependem através de
// "curl -sf http://localhost:7070/monitoring/health/quick" — degraded
// precisa responder 200 (senão toda instalação nova, com WXS opcional
// degradado, volta a reportar ❌ pra sempre, o bug das Rounds 2/3) e
// unhealthy precisa responder 503 (senão uma aplicação de fato quebrada
// nunca seria pega, e o script mentiria ✅).
func TestQuickHealthCheck_ContratoHTTP(t *testing.T) {
	casos := []struct {
		nome            string
		status          monitoring.HealthStatus
		statusHTTPQuero int
	}{
		{"degraded responde 200", monitoring.HealthStatusDegraded, http.StatusOK},
		{"unhealthy responde 503", monitoring.HealthStatusUnhealthy, http.StatusServiceUnavailable},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			hm := monitoring.NewHealthMonitor()
			hm.RegisterChecker(&checkerFixo{nome: "componente", status: c.status})

			h := &Handler{healthMonitor: hm}
			r := gin.New()
			r.GET("/monitoring/health/quick", h.quickHealthCheck)

			req := httptest.NewRequest(http.MethodGet, "/monitoring/health/quick", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != c.statusHTTPQuero {
				t.Errorf("HTTP status = %d, quero %d — corpo: %s", w.Code, c.statusHTTPQuero, w.Body.String())
			}
		})
	}
}

func TestAdvancedHealthCheck_ContratoHTTP(t *testing.T) {
	casos := []struct {
		nome            string
		status          monitoring.HealthStatus
		statusHTTPQuero int
	}{
		{"degraded responde 200", monitoring.HealthStatusDegraded, http.StatusOK},
		{"unhealthy responde 503", monitoring.HealthStatusUnhealthy, http.StatusServiceUnavailable},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			hm := monitoring.NewHealthMonitor()
			hm.RegisterChecker(&checkerFixo{nome: "componente", status: c.status})

			h := &Handler{healthMonitor: hm}
			r := gin.New()
			r.GET("/monitoring/health", h.advancedHealthCheck)

			req := httptest.NewRequest(http.MethodGet, "/monitoring/health", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != c.statusHTTPQuero {
				t.Errorf("HTTP status = %d, quero %d — corpo: %s", w.Code, c.statusHTTPQuero, w.Body.String())
			}
		})
	}
}

// TestQuickHealthCheck_UsaCache confirma que a segunda chamada realmente
// veio do caminho cacheado (getCachedResponse), não de um novo CheckAll —
// checando o marcador "cached": true que só getCachedResponse escreve no
// payload. A extração de aggregateOverallStatus (feita nesta mesma rodada)
// fez CheckAll e getCachedResponse convergirem pra uma única regra de
// agregação; sem um teste passando pelo cache de verdade, uma regressão
// introduzida só na cópia cacheada — como o bug original de 503 permanente —
// passaria despercebida sempre que a resposta viesse do cache, que é a
// maior parte do tempo em produção graças ao cache de 5s.
func TestQuickHealthCheck_UsaCache(t *testing.T) {
	hm := monitoring.NewHealthMonitor()
	hm.RegisterChecker(&checkerFixo{nome: "wxs_db", status: monitoring.HealthStatusDegraded})

	h := &Handler{healthMonitor: hm}
	r := gin.New()
	r.GET("/monitoring/health/quick", h.quickHealthCheck)

	// Primeira chamada: cache ainda vazio, QuickCheck cai em CheckAll, que
	// termina populando o cache via updateCache.
	req1 := httptest.NewRequest(http.MethodGet, "/monitoring/health/quick", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("primeira chamada: HTTP status = %d, quero 200 — corpo: %s", w1.Code, w1.Body.String())
	}

	// Segunda chamada, dentro da janela de 5s do cache: deve vir de
	// getCachedResponse.
	req2 := httptest.NewRequest(http.MethodGet, "/monitoring/health/quick", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("segunda chamada (cache): HTTP status = %d, quero 200 — corpo: %s", w2.Code, w2.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v — corpo: %s", err, w2.Body.String())
	}
	if cached, _ := body["cached"].(bool); !cached {
		t.Errorf("segunda chamada não veio do cache (\"cached\" ausente ou false) — corpo: %s", w2.Body.String())
	}
	if status, _ := body["status"].(string); status != string(monitoring.HealthStatusDegraded) {
		t.Errorf("status no payload cacheado = %q, quero %q", status, monitoring.HealthStatusDegraded)
	}
}
