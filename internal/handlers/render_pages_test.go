package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRenderTodasAsPaginas renderiza cada página com um contexto
// representativo e confirma que o HTML fecha. Um erro de execução de
// template (campo inexistente, tipo errado num helper) não quebra o build
// nem o parse — ele aborta a renderização no meio, e a página chega
// truncada ao browser. Sem este teste, isso só apareceria em runtime.
func TestRenderTodasAsPaginas(t *testing.T) {
	h := &Handler{templates: buildTemplateCache(), appVersion: "1.4"}

	casos := []struct {
		pagina string
		dados  gin.H
	}{
		{"devices.html", gin.H{
			"devices": []map[string]interface{}{{
				"lc_id": 123, "name": "Portaria Norte", "model": "hikvision",
				"port": 7070, "log_enabled": 1, "status": "running",
				"interval": 30, "total": 412,
			}},
			"page": 1, "total_pages": 3, "per_page": 10,
			"page_range":    []int{1, 2, 3},
			"counter_cards": FleetCounts{Total: 3, Running: 1, Stopped: 1, Disabled: 1}.toMap(),
			"filters":       map[string]string{"id": "", "name": "", "port": ""},
		}},
		{"comparison.html", gin.H{
			"values": []map[string]interface{}{{
				"site_controller_id": 1, "local_controller_id": 123, "name": "Doca",
				"port": 7071, "wxs_total": 10, "site_controller_total": 10, "emulator_total": 9,
			}},
			"page": 1, "total_pages": 2, "per_page": 10, "page_range": []int{1, 2},
			"counter_cards": FleetCounts{}.toMap(),
		}},
		{"settings.html", gin.H{
			"wxs_settings": struct {
				Host, Database, Username, Password string
				Port                               int
			}{"10.0.0.1", "W_Access", "sa", "segredo", 1433},
		}},
		{"error.html", gin.H{"error": "conexão recusada"}},
	}

	for _, caso := range casos {
		t.Run(caso.pagina, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			h.renderPage(c, caso.pagina, http.StatusOK, caso.dados)

			corpo := rec.Body.String()
			if !strings.Contains(corpo, "</html>") {
				t.Fatalf("render de %s foi truncado — o HTML não fecha:\n%s", caso.pagina, ultimosBytes(corpo, 800))
			}
			if !strings.Contains(corpo, "fleet-meter") {
				t.Errorf("%s: o header não renderizou", caso.pagina)
			}
		})
	}
}

// ultimosBytes devolve o fim da string, para a mensagem de falha mostrar
// onde a renderização parou.
func ultimosBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
