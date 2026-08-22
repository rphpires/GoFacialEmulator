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
