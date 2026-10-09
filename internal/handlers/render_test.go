package handlers

import (
	"net/http"
	"net/http/httptest"
	"regexp"
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

// TestRenderDevices_ColunaModo garante o cabeçalho da coluna Modo. O
// seletor por linha (só Dahua) é desenhado por devices.js a partir do
// campo mode da frota.
func TestRenderDevices_ColunaModo(t *testing.T) {
	corpo := renderizarDevices(t, []deviceView{{ID: 1, Model: "Dahua", Mode: modoStandalone}})

	if !strings.Contains(corpo, "<th>Modo</th>") {
		t.Errorf("a grade não tem cabeçalho da coluna Modo:\n%s", corpo)
	}
	if !strings.Contains(corpo, `"mode":"standalone"`) {
		t.Errorf("o modo gravado não veio na frota embutida:\n%s", corpo)
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
		"fleet":         []deviceView{},
		"counter_cards": FleetCounts{}.toMap(),
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

// TestRenderDevices_BotoesEmLote garante que as ações em lote nascem
// escondidas dentro da barra de seleção. Visíveis sem seleção elas
// disparariam o confirm de uma operação irreversível sobre nada — e o
// devices.js só revela a barra quando há dispositivo marcado.
func TestRenderDevices_BotoesEmLote(t *testing.T) {
	corpo := renderizarDevices(t, nil)

	re := regexp.MustCompile(`(?s)<div class="bulk-bar" id="bulk-bar"[^>]*hidden>(.*?)</div>`)
	m := re.FindStringSubmatch(corpo)
	if m == nil {
		t.Fatalf("barra de ações em lote ausente ou já visível:\n%s", corpo)
	}
	for _, id := range []string{"start-selected", "stop-selected", "delete-selected"} {
		if !strings.Contains(m[1], `id="`+id+`"`) {
			t.Errorf("botão em lote %q fora da barra de seleção", id)
		}
	}
}
