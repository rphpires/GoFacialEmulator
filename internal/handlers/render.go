package handlers

import (
	"html/template"
	"net/http"

	"GoFacialEmulator/assets"

	"github.com/gin-gonic/gin"
)

// pageTemplates são as páginas que o shell base.html envolve. Toda página
// nova precisa entrar aqui, senão renderPage recebe nil no lookup.
var pageTemplates = []string{
	"devices.html",
	"comparison.html",
	"settings.html",
	"error.html",
}

// shellTemplates são os parciais que compõem o shell, parseados junto com
// cada página.
var shellTemplates = []string{
	"web/templates/base.html",
	"web/templates/header.html",
	"web/templates/footer.html",
	"web/templates/sidebar.html",
}

// templateFuncs são os helpers disponíveis dentro dos templates.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"gt":  func(a, b int) bool { return a > b },
		"eq":  func(a, b interface{}) bool { return a == b },
		"ne":  func(a, b interface{}) bool { return a != b },
		"default": func(defaultValue interface{}, value interface{}) interface{} {
			if value == nil {
				return defaultValue
			}
			return value
		},
	}
}

// buildTemplateCache parseia todas as páginas uma única vez, no startup.
// Antes cada request re-parseava seis arquivos do FS embutido; como os
// templates são embedados, eles não mudam em runtime e reparsear era puro
// desperdício. Faz panic em template inválido de propósito: é erro de
// programação, e falhar no boot é melhor que falhar no primeiro acesso.
func buildTemplateCache() map[string]*template.Template {
	cache := make(map[string]*template.Template, len(pageTemplates))

	for _, page := range pageTemplates {
		files := append(append([]string{}, shellTemplates...), "web/templates/"+page)
		cache[page] = template.Must(
			template.New("").Funcs(templateFuncs()).ParseFS(assets.Templates(), files...),
		)
	}

	return cache
}

// renderPage escreve uma página completa na resposta. Substitui c.HTML,
// que exige um HTMLRender configurado no gin — este app nunca configurou
// um, e por isso os c.HTML de erro davam panic em vez de página.
func (h *Handler) renderPage(c *gin.Context, name string, status int, data gin.H) {
	tmpl := h.templates[name]
	if tmpl == nil {
		h.tracer.Error("Template %q ausente do cache", name)
		c.String(http.StatusInternalServerError, "Template indisponível: %s", name)
		return
	}

	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")

	ctx := h.withBaseContext(data)

	// O header é comum a todas as páginas e lê counter_cards. settingsPage
	// nunca passou a chave e error.html também não tem de onde tirá-la:
	// sem este default, a renderização aborta no meio do header e a página
	// sai truncada. O erro passava despercebido porque o retorno de
	// ExecuteTemplate era descartado nos três pontos de render.
	if _, ok := ctx["counter_cards"]; !ok {
		ctx["counter_cards"] = FleetCounts{}.toMap()
	}

	// O erro é registrado, não engolido: uma página truncada precisa
	// aparecer no log em vez de virar mistério de suporte.
	if err := tmpl.ExecuteTemplate(c.Writer, "base.html", ctx); err != nil {
		h.tracer.Error("Failed to render %q: %v", name, err)
	}
}
