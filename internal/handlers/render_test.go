package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRenderPage_Erro cobre o buraco que fazia qualquer falha de banco
// virar 500 em branco: os handlers chamavam c.HTML(500, "error.html"),
// mas error.html não existia e o HTMLRender do gin nunca foi configurado
// (este app renderiza via tmpl.ExecuteTemplate direto). O resultado era
// panic, capturado pelo RecoveryMiddleware, e nenhuma explicação na tela.
func TestRenderPage_Erro(t *testing.T) {
	h := &Handler{
		templates:  buildTemplateCache(),
		appVersion: "teste",
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.renderPage(c, "error.html", http.StatusInternalServerError, gin.H{
		"error": "conexão recusada pelo banco",
	})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, quero %d", rec.Code, http.StatusInternalServerError)
	}

	corpo := rec.Body.String()
	if !strings.Contains(corpo, "conexão recusada pelo banco") {
		t.Errorf("a mensagem de erro não chegou no HTML renderizado:\n%s", corpo)
	}
	if !strings.Contains(corpo, "Facial Emulators") {
		t.Errorf("a página de erro deveria vir dentro do shell da aplicação:\n%s", corpo)
	}
}

// TestBuildTemplateCache_CobreTodasAsPáginas garante que nenhuma página
// fique de fora do cache montado no startup — uma página ausente aqui só
// apareceria como nil pointer em produção, no primeiro acesso.
func TestBuildTemplateCache_CobreTodasAsPáginas(t *testing.T) {
	cache := buildTemplateCache()

	for _, nome := range []string{"devices.html", "comparison.html", "settings.html", "error.html"} {
		if cache[nome] == nil {
			t.Errorf("template %q ausente do cache", nome)
		}
	}
}

// TestRenderPage_EscapaHTML trava a troca de text/template para
// html/template. Os nomes de dispositivo vêm do banco do W-Access e são
// interpolados direto no HTML; com text/template um nome contendo markup
// era injetado cru na página. O código antigo contornava o problema em vez
// de corrigi-lo (ver o comentário no topo de device-details.js), e sem
// este teste a regressão passaria despercebida.
func TestRenderPage_EscapaHTML(t *testing.T) {
	h := &Handler{
		templates:  buildTemplateCache(),
		appVersion: "teste",
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.renderPage(c, "error.html", http.StatusInternalServerError, gin.H{
		"error": `<script>alert(1)</script>`,
	})

	corpo := rec.Body.String()
	if strings.Contains(corpo, "<script>alert(1)</script>") {
		t.Errorf("markup do payload saiu sem escaping — html/template não está em uso:\n%s", corpo)
	}
	if !strings.Contains(corpo, "&lt;script&gt;") {
		t.Errorf("o texto escapado não apareceu na página:\n%s", corpo)
	}
}

// TestRenderDevices_ColunaModo trava a coluna Modo na interface
// redesenhada. Ela nasceu em feat/rede-e-modo-dispositivo escrita em
// Bootstrap sobre um main.js que não existe mais; o merge não a traz de
// volta sozinha, e sem ela o capítulo do manual descreve uma coluna
// invisível.
func TestRenderDevices_ColunaModo(t *testing.T) {
	h := &Handler{
		templates:  buildTemplateCache(),
		appVersion: "teste",
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.renderPage(c, "devices.html", http.StatusOK, gin.H{
		"devices": []gin.H{
			{"lc_id": 1, "name": "Portaria", "model": "Dahua", "port": 4001,
				"status": "running", "log_enabled": 0, "interval": 30,
				"total": 12, "local_auth": "standalone"},
			{"lc_id": 2, "name": "Garagem", "model": "Hikvision", "port": 4002,
				"status": "stopped", "log_enabled": 0, "interval": 30,
				"total": 8, "local_auth": "online"},
		},
		"filters":       gin.H{"id": "", "name": "", "port": ""},
		"page":          1,
		"total_pages":   1,
		"per_page":      50,
		"page_range":    []int{1},
		"counter_cards": FleetCounts{Total: 2, Running: 1, Stopped: 1, Disabled: 0}.toMap(),
	})

	corpo := rec.Body.String()

	if !strings.Contains(corpo, "<th>Modo</th>") {
		t.Errorf("a grade não tem cabeçalho da coluna Modo:\n%s", corpo)
	}
	// Dahua ganha o seletor; os demais modelos ignoram LocalAuthentication e
	// mostram um traço, então um select ali seria um controle que não faz nada.
	if !strings.Contains(corpo, `class="select device-mode" data-device-id="1"`) {
		t.Errorf("o dispositivo Dahua deveria ter o seletor de modo:\n%s", corpo)
	}
	if strings.Contains(corpo, `device-mode" data-device-id="2"`) {
		t.Errorf("o dispositivo Hikvision não deveria ter seletor de modo:\n%s", corpo)
	}
	// O valor gravado tem de vir selecionado, senão a tela mente sobre o
	// estado do dispositivo.
	if !strings.Contains(corpo, `value="standalone" selected`) {
		t.Errorf("o modo gravado não veio selecionado:\n%s", corpo)
	}
}

// TestRenderDevices_BlocoDeAlcancabilidade garante que o contêiner do aviso
// existe e nasce escondido. O aviso só tem conteúdo quando /api/reachability
// responde que há dispositivo inalcançável — um bloco visível e vazio seria
// pior que nenhum.
func TestRenderDevices_BlocoDeAlcancabilidade(t *testing.T) {
	h := &Handler{
		templates:  buildTemplateCache(),
		appVersion: "teste",
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.renderPage(c, "devices.html", http.StatusOK, gin.H{
		"devices":       []gin.H{},
		"filters":       gin.H{"id": "", "name": "", "port": ""},
		"page":          1,
		"total_pages":   1,
		"per_page":      50,
		"page_range":    []int{1},
		"counter_cards": FleetCounts{Total: 0, Running: 0, Stopped: 0, Disabled: 0}.toMap(),
	})

	corpo := rec.Body.String()

	for _, id := range []string{
		`id="reachability-alert-row"`,
		`id="reachability-alert"`,
		`id="reachability-headline"`,
		`id="reachability-reason"`,
		`id="reachability-list"`,
		`id="reachability-toggle"`,
	} {
		if !strings.Contains(corpo, id) {
			t.Errorf("marcador ausente na página de dispositivos: %s\n%s", id, corpo)
		}
	}
	if !strings.Contains(corpo, `id="reachability-alert-row" hidden`) {
		t.Errorf("o bloco de aviso deveria nascer escondido:\n%s", corpo)
	}
	// A interface nova não tem Bootstrap. Classe de framework aqui significa
	// que o HTML antigo foi colado em vez de reescrito.
	for _, proibida := range []string{"alert-warning", "btn-link", "text-muted", "bi bi-"} {
		if strings.Contains(corpo, proibida) {
			t.Errorf("classe de Bootstrap reintroduzida (%q) — reescreva nos componentes do console:\n%s", proibida, corpo)
		}
	}
}

// TestRenderDevices_BotoesEmLote garante que o botão de exclusão em lote
// nasce desabilitado, como os outros dois. Habilitado sem seleção ele
// dispararia o confirm de uma operação irreversível sobre nada — e o
// devices.js só reabilita quando há linha marcada.
func TestRenderDevices_BotoesEmLote(t *testing.T) {
	h := &Handler{
		templates:  buildTemplateCache(),
		appVersion: "teste",
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.renderPage(c, "devices.html", http.StatusOK, gin.H{
		"devices":       []gin.H{},
		"filters":       gin.H{"id": "", "name": "", "port": ""},
		"page":          1,
		"total_pages":   1,
		"per_page":      50,
		"page_range":    []int{1},
		"counter_cards": FleetCounts{Total: 0, Running: 0, Stopped: 0, Disabled: 0}.toMap(),
	})

	corpo := rec.Body.String()

	for _, id := range []string{"start-selected", "stop-selected", "delete-selected"} {
		marcador := `id="` + id + `" disabled`
		if !strings.Contains(corpo, marcador) {
			t.Errorf("botão em lote %q ausente ou já habilitado:\n%s", id, corpo)
		}
	}
}
