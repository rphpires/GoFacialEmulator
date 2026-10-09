package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"GoFacialEmulator/internal/database"

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
			"fleet": []deviceView{{
				ID: 123, Name: "Portaria Norte", Model: "Hikvision", Port: 7070,
				LogEnabled: 1, Status: "running", Interval: 30, TotalUsers: 412,
				Source: "wxs", Mode: modoOnline,
			}},
			"counter_cards": FleetCounts{Total: 3, Running: 1, Stopped: 1, Disabled: 1}.toMap(),
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

// renderizarPagina renderiza um template com o contexto dado e devolve o
// HTML. Complementa TestRenderTodasAsPaginas, que só confere que a página
// fecha; aqui os testes olham o conteúdo.
func renderizarPagina(t *testing.T, pagina string, dados gin.H) string {
	t.Helper()

	h := &Handler{templates: buildTemplateCache(), appVersion: "teste"}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.renderPage(c, pagina, http.StatusOK, dados)
	return rec.Body.String()
}

// renderizarDevices monta o contexto mínimo que devices.html exige.
func renderizarDevices(t *testing.T, fleet []deviceView) string {
	t.Helper()
	return renderizarPagina(t, "devices.html", gin.H{
		"fleet":         fleet,
		"counter_cards": FleetCounts{}.toMap(),
	})
}

// A tabela é desenhada no cliente; o que o servidor garante é a frota
// embutida como JSON válido para a primeira pintura.
func TestDevicesHTMLEmbuteAFrotaComoJSON(t *testing.T) {
	html := renderizarDevices(t, []deviceView{{
		ID: 900001, Name: "lab </script> 4000", Model: "Dahua", Port: 4000,
		Status: "stopped", Enabled: 1, Interval: 10, Source: "manual", Mode: modoStandalone,
	}})

	re := regexp.MustCompile(`(?s)<script type="application/json" id="fleet-initial">(.*?)</script>`)
	m := re.FindStringSubmatch(html)
	if m == nil {
		t.Fatal("não encontrei o <script id=\"fleet-initial\">")
	}

	var frota []deviceView
	if err := json.Unmarshal([]byte(m[1]), &frota); err != nil {
		t.Fatalf("frota embutida não é JSON válido: %v\n%s", err, m[1])
	}
	if len(frota) != 1 || frota[0].ID != 900001 || frota[0].Source != "manual" {
		t.Errorf("frota embutida = %+v", frota)
	}
	if frota[0].Name != "lab </script> 4000" {
		t.Errorf("nome com </script> não sobreviveu ao escape: %q", frota[0].Name)
	}
}

func TestDevicesHTMLTemBotoesDeCadastro(t *testing.T) {
	html := renderizarDevices(t, nil)

	for _, id := range []string{
		"new-emulator", "new-emulator-range", "emulator-form-modal",
		"device-rows", "bulk-bar", "filter-q", "pager-pages",
	} {
		if !strings.Contains(html, id) {
			t.Errorf("quero o elemento %q na página", id)
		}
	}
}

// renderizarSettings monta o contexto mínimo que settings.html exige.
func renderizarSettings(t *testing.T, dados gin.H) string {
	t.Helper()
	return renderizarPagina(t, "settings.html", dados)
}

// tagDoToggleDeSync extrai só a tag <input ... id="sync-enabled" ...> do
// HTML renderizado, para o teste checar "checked" presa a esse elemento —
// e não a qualquer outra checkbox marcada na página.
func tagDoToggleDeSync(t *testing.T, html string) string {
	t.Helper()
	re := regexp.MustCompile(`<input[^>]*id="sync-enabled"[^>]*>`)
	tag := re.FindString(html)
	if tag == "" {
		t.Fatal("não encontrei o <input id=\"sync-enabled\"> na página")
	}
	return tag
}

func TestSettingsHTMLTemToggleDeSync(t *testing.T) {
	html := renderizarSettings(t, gin.H{
		"wxs_settings": &database.WxsSettings{Host: "10.0.0.2", Port: 1433},
		"sync_enabled": true,
	})

	if !strings.Contains(html, "sync-enabled") {
		t.Error("quero o toggle de sincronização na tela")
	}

	tag := tagDoToggleDeSync(t, html)
	if !strings.Contains(tag, "checked") {
		t.Errorf("sync ligado tem que vir marcado, tag: %s", tag)
	}
}

func TestSettingsHTMLTemToggleDeSyncDesligado(t *testing.T) {
	html := renderizarSettings(t, gin.H{
		"wxs_settings": &database.WxsSettings{Host: "10.0.0.2", Port: 1433},
		"sync_enabled": false,
	})

	tag := tagDoToggleDeSync(t, html)
	if strings.Contains(tag, "checked") {
		t.Errorf("sync desligado não pode vir marcado, tag: %s", tag)
	}
}
