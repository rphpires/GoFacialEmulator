package handlers

import (
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
			"devices": []map[string]interface{}{{
				"lc_id": 123, "name": "Portaria Norte", "model": "hikvision",
				"port": 7070, "log_enabled": 1, "status": "running",
				"interval": 30, "total": 412, "source": "wxs",
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
func renderizarDevices(t *testing.T, devices []map[string]interface{}) string {
	t.Helper()
	return renderizarPagina(t, "devices.html", gin.H{
		"devices":       devices,
		"page":          1,
		"total_pages":   1,
		"per_page":      10,
		"page_range":    []int{1},
		"counter_cards": FleetCounts{}.toMap(),
		"filters":       map[string]string{"id": "", "name": "", "port": ""},
	})
}

func TestDevicesHTMLMostraOrigem(t *testing.T) {
	html := renderizarDevices(t, []map[string]interface{}{
		{
			"lc_id": 900001, "name": "lab-4000", "ip_address": "127.0.0.1",
			"port": 4000, "log_enabled": 0, "model": "Dahua", "status": "stopped",
			"enabled": 1, "interval": 10, "total": 0, "local_auth": "standalone",
			"source": "manual",
		},
	})

	if !strings.Contains(html, "Manual") {
		t.Error("quero o badge de origem manual na grade")
	}
	if !strings.Contains(html, "device-remove") {
		t.Error("quero o botão de remover em dispositivo manual")
	}
}

func TestDevicesHTMLTemBotoesDeCadastro(t *testing.T) {
	html := renderizarDevices(t, nil)

	for _, id := range []string{"new-emulator", "new-emulator-range", "emulator-form-modal"} {
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

func TestDevicesHTMLNaoOfereceRemoverEmDispositivoDoWXS(t *testing.T) {
	html := renderizarDevices(t, []map[string]interface{}{
		{
			"lc_id": 17, "name": "Portaria", "ip_address": "10.0.0.7",
			"port": 7070, "log_enabled": 0, "model": "Hikvision", "status": "stopped",
			"enabled": 1, "interval": 10, "total": 3, "local_auth": "online",
			"source": "wxs",
		},
	})

	if strings.Contains(html, "device-remove") {
		t.Error("dispositivo do W-Access não pode oferecer remoção")
	}
}
