# Console UI Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remodelar a interface web do emulador como um console de bancada escuro, com uma camada de tempo real que de fato funciona e zero dependência de CDN.

**Architecture:** O backend Go (gin + `html/template` + `go:embed`) ganha cache de templates no startup, contagens de frota com três estados e um stream SSE que entrega snapshot no connect e mantém keepalive. O front abandona Bootstrap/jQuery por um sistema de design próprio em CSS com tokens, servido pelo embed já existente, e consome o stream por uma conexão única com pub/sub.

**Tech Stack:** Go 1.x, gin, `html/template`, `go:embed`, CSS moderno (custom properties, grid, `:has`), JavaScript sem framework nem build step, IBM Plex Sans/Mono como `woff2` locais.

**Spec:** `docs/superpowers/specs/2026-08-22-console-ui-redesign-design.md`

## Global Constraints

- **Zero requisição externa em runtime.** Nenhum `https://` em `assets/web/templates/` ou `assets/web/static/` ao final. Fontes, ícones e CSS são servidos pelo `go:embed`.
- **Nenhum `<style>` ou `<script>` inline nos templates.** Todo CSS em `assets/web/static/css/`, todo JS em `assets/web/static/js/`.
- **Bootstrap, Bootstrap Icons, jQuery e Font Awesome saem por completo.** Nenhuma classe `col-*`, `d-flex`, `btn-*`, `bi-*` sobrevive nos templates ao final.
- **Sem build step.** Nada de npm, bundler ou transpilação. Os arquivos servidos são os arquivos escritos.
- **Tema escuro único.** Todos os tokens em um único bloco `:root`. Sem blocos `prefers-color-scheme`.
- **Laranja (`--signal`, `#ff8c00`) nunca comunica estado.** Só ação e foco. Estado é `--live` / `--halt` / `--idle`.
- **Idioma da interface: pt-BR.** Comentários de código em português, seguindo o padrão do repositório.
- **`prefers-reduced-motion: reduce` respeitado** em toda animação introduzida.
- **Testes Go em português**, no estilo de `internal/handlers/health_http_test.go` (`httptest`, subtests com `t.Run`, mensagens explicando a decisão coberta).
- **Templates usam `html/template`**, nunca `text/template`. A Task 2 faz a troca; nenhuma tarefa posterior pode revertê-la para "resolver" escaping indesejado.
- **Nada de HTML por concatenação de string no JS.** Linhas de tabela e mensagens são nós construídos com `document.createElement` e `textContent`. Dados de dispositivo e de usuário vêm do banco do W-Access.
- **Funcionalidade existente não regride.** O detalhe de dispositivo (usuários e configurações) já está implementado e em uso; a Task 8 o porta para o novo design com paridade obrigatória, item a item. Melhorar é permitido, remover não.
- Ao final de cada tarefa: `go build ./...` e `go test ./...` devem passar.

---

### Task 1: Contagens de frota com três estados e intervalo de paginação

O header hoje mostra dois estados enquanto a tabela mostra três (`disabled`
some dentro de `stopped`), e os templates iteram `page_range`, que nenhum
handler define — por isso os números de página nunca aparecem. Ambos são
dados puros, sem I/O, então são testáveis de forma isolada e vêm primeiro.

**Files:**
- Create: `internal/handlers/fleet.go`
- Create: `internal/handlers/fleet_test.go`
- Modify: `internal/handlers/handlers.go` (`mainPage`, `comparisonPage`, `getSystemStatus`)

**Interfaces:**
- Produces: `type FleetCounts struct { Total, Running, Stopped, Disabled int }`
- Produces: `func countFleet(devices []models.Device) FleetCounts`
- Produces: `func paginationRange(page, totalPages int) []int`
- Produces: `func (f FleetCounts) toMap() map[string]int`

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/handlers/fleet.go` ainda não; primeiro o teste.

```go
package handlers

import (
	"reflect"
	"testing"

	"GoFacialEmulator/internal/models"
)

// TestCountFleet cobre a decisão que hoje faz o header divergir da tabela:
// um dispositivo com Enabled == 0 é "disabled" mesmo que o Status gravado
// diga outra coisa, e nunca deve ser contado como stopped. A tabela já
// aplica essa regra (handlers.go, getCurrentDevicesWithFilters); os
// contadores não aplicavam.
func TestCountFleet(t *testing.T) {
	casos := []struct {
		nome    string
		devices []models.Device
		quero   FleetCounts
	}{
		{
			nome:    "frota vazia",
			devices: nil,
			quero:   FleetCounts{Total: 0, Running: 0, Stopped: 0, Disabled: 0},
		},
		{
			nome: "desabilitado não conta como parado",
			devices: []models.Device{
				{ID: 1, Enabled: 1, Status: "running"},
				{ID: 2, Enabled: 1, Status: "stopped"},
				{ID: 3, Enabled: 0, Status: "stopped"},
			},
			quero: FleetCounts{Total: 3, Running: 1, Stopped: 1, Disabled: 1},
		},
		{
			nome: "desabilitado com status running ainda é desabilitado",
			devices: []models.Device{
				{ID: 1, Enabled: 0, Status: "running"},
			},
			quero: FleetCounts{Total: 1, Running: 0, Stopped: 0, Disabled: 1},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if tem := countFleet(caso.devices); tem != caso.quero {
				t.Errorf("countFleet() = %+v, quero %+v", tem, caso.quero)
			}
		})
	}
}

// TestPaginationRange cobre o bug que deixava a paginação sem números:
// os templates iteram .page_range e nenhum handler definia a chave, então
// o range corria sobre nil e só sobravam "Anterior"/"Próxima".
func TestPaginationRange(t *testing.T) {
	casos := []struct {
		nome       string
		page       int
		totalPages int
		quero      []int
	}{
		{"página única", 1, 1, []int{1}},
		{"sem páginas devolve vazio", 1, 0, []int{}},
		{"poucas páginas mostra todas", 2, 5, []int{1, 2, 3, 4, 5}},
		{"janela no começo", 1, 20, []int{1, 2, 3, 4, 5, 6, 7}},
		{"janela no meio", 10, 20, []int{7, 8, 9, 10, 11, 12, 13}},
		{"janela no fim", 20, 20, []int{14, 15, 16, 17, 18, 19, 20}},
		{"página fora do intervalo é fixada no limite", 99, 3, []int{1, 2, 3}},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			tem := paginationRange(caso.page, caso.totalPages)
			if !reflect.DeepEqual(tem, caso.quero) {
				t.Errorf("paginationRange(%d, %d) = %v, quero %v",
					caso.page, caso.totalPages, tem, caso.quero)
			}
		})
	}
}
```

- [ ] **Step 2: Rodar os testes e confirmar que falham**

Run: `go test ./internal/handlers/ -run "TestCountFleet|TestPaginationRange" -v`
Expected: FAIL na compilação, com `undefined: FleetCounts`, `undefined: countFleet`, `undefined: paginationRange`.

- [ ] **Step 3: Implementar `internal/handlers/fleet.go`**

```go
package handlers

import "GoFacialEmulator/internal/models"

// paginationWindow é quantos números de página a barra mostra de uma vez.
// Ímpar de propósito: a página atual fica centrada, com a mesma quantidade
// de vizinhos de cada lado.
const paginationWindow = 7

// FleetCounts agrega a frota nos três estados que a tabela de dispositivos
// já distingue. Antes o header agregava só dois e "disabled" era somado a
// "stopped", o que fazia o topo da página discordar da própria tabela
// logo abaixo.
type FleetCounts struct {
	Total    int `json:"total"`
	Running  int `json:"running"`
	Stopped  int `json:"stopped"`
	Disabled int `json:"disabled"`
}

// countFleet classifica cada dispositivo em exatamente um estado.
// Enabled == 0 vence o Status gravado: um dispositivo desabilitado no
// W-Access não está "parado", está fora de operação, e a UI precisa
// mostrar essa diferença porque as ações disponíveis mudam.
func countFleet(devices []models.Device) FleetCounts {
	counts := FleetCounts{Total: len(devices)}

	for _, device := range devices {
		switch {
		case device.Enabled == 0:
			counts.Disabled++
		case device.Status == "running":
			counts.Running++
		default:
			counts.Stopped++
		}
	}

	return counts
}

// toMap devolve as contagens no formato que os templates consomem via
// {{ .counter_cards.running }}.
func (f FleetCounts) toMap() map[string]int {
	return map[string]int{
		"total":    f.Total,
		"running":  f.Running,
		"stopped":  f.Stopped,
		"disabled": f.Disabled,
	}
}

// paginationRange devolve os números de página a renderizar, centrados na
// página atual e limitados a paginationWindow. Devolve sempre uma fatia
// não-nil para o template poder iterar sem checagem.
func paginationRange(page, totalPages int) []int {
	if totalPages < 1 {
		return []int{}
	}

	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := page - paginationWindow/2
	if start < 1 {
		start = 1
	}

	end := start + paginationWindow - 1
	if end > totalPages {
		end = totalPages
		start = end - paginationWindow + 1
		if start < 1 {
			start = 1
		}
	}

	pages := make([]int, 0, end-start+1)
	for p := start; p <= end; p++ {
		pages = append(pages, p)
	}

	return pages
}
```

- [ ] **Step 4: Rodar os testes e confirmar que passam**

Run: `go test ./internal/handlers/ -run "TestCountFleet|TestPaginationRange" -v`
Expected: PASS em todos os subtests.

- [ ] **Step 5: Ligar no `mainPage`**

Em `internal/handlers/handlers.go`, dentro de `mainPage`, substituir o
bloco que monta `counterCards` e `context`. Localizar:

```go
	// Contadores baseados em TODOS os dispositivos
	totalAllDevices := len(allDevices)
	counterCards := map[string]interface{}{
		"total":   totalAllDevices,
		"running": deviceStatusRunning,
		"stopped": totalAllDevices - deviceStatusRunning,
	}

	context := gin.H{
		"devices":       paginatedDevices,
		"page":          page,
		"total_pages":   totalPages,
		"per_page":      perPage,
		"counter_cards": counterCards,
		"filters":       filters,
	}
```

e trocar por:

```go
	// Contadores vêm de countFleet para o header concordar com a tabela:
	// desabilitado é um estado próprio, não um "parado".
	fleetDevices, err := h.manager.ListDevices()
	if err != nil {
		h.tracer.Error("Failed to list devices for counters: %v", err)
		fleetDevices = nil
	}
	counts := countFleet(fleetDevices)

	context := gin.H{
		"devices":       paginatedDevices,
		"page":          page,
		"total_pages":   totalPages,
		"per_page":      perPage,
		"page_range":    paginationRange(page, totalPages),
		"counter_cards": counts.toMap(),
		"filters":       filters,
	}
```

As variáveis `allDevices` e `deviceStatusRunning` ficam sem uso a partir
daqui. Substituir a chamada

```go
	allDevices, deviceStatusRunning, err := h.getCurrentDevices()
```

e o bloco de erro logo abaixo dela por nada — a linha
`h.tracer.Info("DEBUG: Filtered devices: %d, All devices: %d", ...)` também
sai, junto com a referência a `allDevices`.

- [ ] **Step 6: Ligar no `comparisonPage` e no `getSystemStatus`**

Em `comparisonPage`, adicionar `"page_range": paginationRange(page, totalPages),`
ao `gin.H` do contexto, ao lado de `"total_pages"`.

Em `getSystemStatus`, substituir o corpo após o `ListDevices` por:

```go
	counts := countFleet(devices)

	c.JSON(http.StatusOK, gin.H{
		"total_devices":    counts.Total,
		"running_devices":  counts.Running,
		"stopped_devices":  counts.Stopped,
		"disabled_devices": counts.Disabled,
		"timestamp":        time.Now().UTC(),
	})
```

O laço manual `runningCount` acima dele sai.

- [ ] **Step 7: Verificar build e suíte completa**

Run: `go build ./... && go test ./... 2>&1 | tail -20`
Expected: build sem saída, testes PASS ou `no test files` por pacote.

- [ ] **Step 8: Commit**

```bash
git add internal/handlers/fleet.go internal/handlers/fleet_test.go internal/handlers/handlers.go
git commit -m "feat(ui): contagens de frota com três estados e page_range real"
```

---

### Task 2: Cache de templates, escaping, página de erro e limpeza

Três problemas no mesmo lugar:

`loadTemplate()` re-parseia seis arquivos do embed a cada request; os
quatro `c.HTML(500, "error.html", ...)` referenciam um template que não
existe num renderer que nunca foi configurado — qualquer falha de banco
vira panic e 500 branco; e `handlers.go` importa **`text/template`**, não
`html/template`.

O terceiro é o mais sério e não estava no levantamento inicial. Com
`text/template` nada é escapado: o nome de um dispositivo, que vem do banco
do W-Access, é interpolado cru no HTML e em atributos `onclick=`. Um nome
com `<script>` ou com aspas executa. O código atual reconhece o problema e
desenha em volta dele — `device-details.js` traz o comentário *"o template
renderiza com text/template (sem escaping), então o nome do dispositivo
nunca entra em contexto JS"* — em vez de corrigir a origem. A troca para
`html/template` é a correção, e ela é segura aqui porque nenhum template
depende de injetar HTML: os `{{ }}` são todos texto e números.

**Files:**
- Create: `assets/web/templates/error.html`
- Create: `internal/handlers/render.go`
- Create: `internal/handlers/render_test.go`
- Modify: `internal/handlers/handlers.go` (struct `Handler`, `NewHandler`, `loadTemplate`, os 4 `c.HTML`)
- Delete: `assets/web/templates/metrics.html`, `assets/web/templates/pagination.html`

**Interfaces:**
- Consumes: `withBaseContext(extra gin.H) gin.H` (já existe, `handlers.go:131`)
- Produces: `func buildTemplateCache() map[string]*template.Template`
- Produces: `func (h *Handler) renderPage(c *gin.Context, name string, status int, data gin.H)`
- Produces: campo `templates map[string]*template.Template` em `Handler`

- [ ] **Step 1: Criar `assets/web/templates/error.html`**

Este template ainda usa o shell antigo; a Task 6 troca o shell inteiro de
uma vez e ele acompanha.

```html
{{ define "title" }}Erro - Facial Emulator Service{{ end }}

{{ define "content" }}
<section class="page-error">
  <h1 class="page-error__title">Não foi possível carregar esta página</h1>
  <p class="page-error__detail">{{ .error }}</p>
  <p class="page-error__hint">
    Verifique a conexão com o banco em <a href="/settings">Configurações</a>,
    ou tente novamente.
  </p>
  <a class="btn btn--action" href="/">Voltar para Dispositivos</a>
</section>
{{ end }}
```

- [ ] **Step 2: Escrever o teste que falha**

Criar `internal/handlers/render_test.go`:

```go
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
```

- [ ] **Step 3: Rodar o teste e confirmar que falha**

Run: `go test ./internal/handlers/ -run "TestRenderPage_Erro|TestBuildTemplateCache" -v`
Expected: FAIL na compilação, com `undefined: buildTemplateCache`, `h.templates undefined`, `h.renderPage undefined`.

- [ ] **Step 4: Implementar `internal/handlers/render.go`**

```go
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

	if err := tmpl.ExecuteTemplate(c.Writer, "base.html", h.withBaseContext(data)); err != nil {
		h.tracer.Error("Failed to render %q: %v", name, err)
	}
}
```

- [ ] **Step 5: Trocar o import e adicionar o campo no `Handler`**

Em `internal/handlers/handlers.go`, trocar na lista de imports:

```go
	"text/template"
```

por:

```go
	"html/template"
```

Essa linha é a correção de escaping. `render.go` (Step 4) já importa
`html/template`; deixar os dois pacotes convivendo faria o tipo
`*template.Template` divergir entre arquivos e não compilaria — a troca é
obrigatória, não opcional.

No `type Handler struct`, acrescentar após `metrics *monitoring.Metrics`:

```go
	templates     map[string]*template.Template
```

Dentro de `NewHandler`, no literal de struct que constrói o `*Handler`,
acrescentar:

```go
		templates:     buildTemplateCache(),
```

- [ ] **Step 6: Rodar os testes e confirmar que passam**

Run: `go test ./internal/handlers/ -run "TestRenderPage|TestBuildTemplateCache" -v`
Expected: PASS nos três testes, incluindo `TestRenderPage_EscapaHTML`.

Confirmar também que `text/template` sumiu do pacote:

Run: `grep -rn "text/template" internal/`
Expected: nenhuma saída.

- [ ] **Step 7: Trocar todos os pontos de render para `renderPage`**

Em `handlers.go`, substituir as quatro chamadas `c.HTML(...)`:

`mainPage` (duas ocorrências, hoje nas linhas 309 e 317):

```go
		h.renderPage(c, "error.html", http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
```

`comparisonPage` (hoje linha 470) e `settingsPage` (hoje linha 1006): mesma
substituição, sendo que a de `settingsPage` mantém a mensagem existente:

```go
		h.renderPage(c, "error.html", http.StatusInternalServerError, gin.H{
			"error": "Erro ao carregar configurações",
		})
		return
```

Substituir também os três pares `tmpl := h.loadTemplate(...)` +
`tmpl.ExecuteTemplate(...)`:

```go
	h.renderPage(c, "devices.html", http.StatusOK, context)
```

```go
	h.renderPage(c, "comparison.html", http.StatusOK, context)
```

```go
	h.renderPage(c, "settings.html", http.StatusOK, gin.H{"wxs_settings": wxsSettings})
```

Depois disso, apagar a função `loadTemplate` inteira de `handlers.go`
(hoje começa na linha ~170 com o `funcMap` e termina no `return template.Must(...)`).

- [ ] **Step 8: Apagar os templates mortos**

```bash
git rm assets/web/templates/metrics.html assets/web/templates/pagination.html
```

Ambos eram parseados em toda página e nunca invocados: `base.html` diz
"Métricas removidas" e as duas tabelas trazem paginação própria inline.
`metrics.html` era também a última referência a Font Awesome e ao plugin
jQuery `countTo`.

- [ ] **Step 9: Verificar build e suíte**

Run: `go build ./... && go test ./... 2>&1 | tail -20`
Expected: build sem saída, testes PASS.

- [ ] **Step 10: Commit**

```bash
git add -A internal/handlers assets/web/templates
git commit -m "refactor(ui): cache de templates no startup, página de erro real, remove templates mortos"
```

---

### Task 3: Stream SSE com snapshot, keepalive e reconexão

O stream só emite em mudança de estado: quem abre a página depois de uma
transição fica com o HTML server-rendered até algo mudar. E sem keepalive
nem `retry:`, uma conexão ociosa morre em proxy sem que o cliente saiba
com que cadência voltar.

**Files:**
- Create: `internal/handlers/stream.go`
- Create: `internal/handlers/stream_test.go`
- Modify: `internal/handlers/handlers.go` (remove `handleSSE`)

**Interfaces:**
- Consumes: `countFleet`, `FleetCounts` (Task 1)
- Consumes: `h.manager.AddStatusListener() emulator.StatusChangeListener`, `h.manager.RemoveStatusListener(...)`, `h.manager.ListDevices() ([]models.Device, error)`
- Produces: `func (h *Handler) handleStream(c *gin.Context)` — registrado em `router.GET("/events", ...)`
- Produces: `type deviceView struct` — forma de um dispositivo no payload SSE, consumida por `realtime.js` (Task 5)
- Produces: `func writeSSE(w io.Writer, event string, payload interface{}) error`

- [ ] **Step 1: Escrever o teste que falha**

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestWriteSSE cobre o formato do wire. É frágil de propósito: o
// EventSource do browser descarta silenciosamente um frame malformado, e
// um "\n" faltando aparece como "tempo real não funciona" sem nenhum erro
// visível dos dois lados.
func TestWriteSSE(t *testing.T) {
	var buf bytes.Buffer

	if err := writeSSE(&buf, "device", map[string]int{"id": 7}); err != nil {
		t.Fatalf("writeSSE devolveu erro: %v", err)
	}

	tem := buf.String()

	if !strings.HasPrefix(tem, "event: device\n") {
		t.Errorf("frame não começa com a linha de evento:\n%q", tem)
	}
	if !strings.HasSuffix(tem, "\n\n") {
		t.Errorf("frame não termina em linha em branco, o EventSource nunca vai despachá-lo:\n%q", tem)
	}

	linhaDados := strings.TrimSuffix(strings.SplitN(tem, "\n", 2)[1], "\n\n")
	if !strings.HasPrefix(linhaDados, "data: ") {
		t.Fatalf("segunda linha não é data:\n%q", tem)
	}

	var decodificado map[string]int
	if err := json.Unmarshal([]byte(strings.TrimPrefix(linhaDados, "data: ")), &decodificado); err != nil {
		t.Fatalf("payload não é JSON válido: %v", err)
	}
	if decodificado["id"] != 7 {
		t.Errorf("payload = %v, quero id 7", decodificado)
	}
}

// TestWriteSSE_PayloadSemQuebraDeLinha garante que o JSON serializado não
// carrega "\n" literal: uma quebra dentro de data: encerraria o frame no
// meio e entregaria JSON truncado ao cliente.
func TestWriteSSE_PayloadSemQuebraDeLinha(t *testing.T) {
	var buf bytes.Buffer

	if err := writeSSE(&buf, "device", map[string]string{"name": "Portaria\nNorte"}); err != nil {
		t.Fatalf("writeSSE devolveu erro: %v", err)
	}

	corpo := strings.TrimSuffix(buf.String(), "\n\n")
	if strings.Count(corpo, "\n") != 1 {
		t.Errorf("frame tem quebra de linha extra dentro do payload:\n%q", buf.String())
	}
}
```

- [ ] **Step 2: Rodar e confirmar que falha**

Run: `go test ./internal/handlers/ -run TestWriteSSE -v`
Expected: FAIL com `undefined: writeSSE`.

- [ ] **Step 3: Implementar `internal/handlers/stream.go`**

```go
package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"GoFacialEmulator/internal/models"

	"github.com/gin-gonic/gin"
)

// streamKeepalive é o intervalo do comentário que mantém a conexão viva.
// Proxies costumam derrubar conexões ociosas em 60s; 20s dá margem de
// três batidas antes disso.
const streamKeepalive = 20 * time.Second

// streamRetry é o que o browser espera antes de reconectar sozinho, em
// milissegundos. Enviado uma vez, na abertura do stream.
const streamRetry = 3000

// deviceView é a forma de um dispositivo no wire do SSE. Espelha as
// colunas que a tabela mostra, para o cliente conseguir redesenhar uma
// linha inteira sem uma segunda requisição.
type deviceView struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	Port       int    `json:"port"`
	Status     string `json:"status"`
	Enabled    int    `json:"enabled"`
	LogEnabled int    `json:"log_enabled"`
	Interval   int    `json:"interval"`
	TotalUsers int    `json:"total_users"`
}

// newDeviceView aplica a mesma regra da tabela: Enabled == 0 vence o
// Status gravado. Manter essa decisão num lugar só evita o header e a
// tabela discordarem, que era exatamente o sintoma antigo.
func newDeviceView(d models.Device) deviceView {
	status := d.Status
	if d.Enabled == 0 {
		status = "disabled"
	}

	return deviceView{
		ID:         d.ID,
		Name:       d.Name,
		Model:      d.Model,
		Port:       d.Port,
		Status:     status,
		Enabled:    d.Enabled,
		LogEnabled: d.LogEnabled,
		Interval:   d.EventInterval,
		TotalUsers: d.TotalUsers,
	}
}

// writeSSE serializa um frame SSE. O JSON vai em linha única porque uma
// quebra dentro de "data:" encerraria o frame no meio.
func writeSSE(w io.Writer, event string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal SSE payload: %w", err)
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

// handleStream serve /events.
//
// Três coisas que a versão anterior não fazia e que apareciam como "o
// tempo real não funciona":
//
//  1. Snapshot no connect. Antes o stream só falava em mudança de estado,
//     então uma aba aberta depois de uma transição ficava com o HTML
//     server-rendered antigo por tempo indeterminado.
//  2. Keepalive. Sem tráfego, um proxy derruba a conexão ociosa; o browser
//     reconecta, mas o ciclo se repetia sem ninguém perceber.
//  3. retry:. Sem ele o browser usa o default próprio, que varia entre
//     implementações.
func (h *Handler) handleStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	// Desliga o buffer do nginx: com ele ligado, os frames ficam presos no
	// proxy e o stream chega em rajadas ou não chega.
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		h.tracer.Error("SSE: ResponseWriter não suporta Flush")
		c.Status(http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintf(c.Writer, "retry: %d\n\n", streamRetry); err != nil {
		return
	}

	listener := h.manager.AddStatusListener()
	defer h.manager.RemoveStatusListener(listener)

	if err := h.writeSnapshot(c.Writer); err != nil {
		h.tracer.Error("SSE: falha ao enviar snapshot: %v", err)
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(streamKeepalive)
	defer ticker.Stop()

	clientGone := c.Request.Context().Done()

	for {
		select {
		case event, ok := <-listener:
			if !ok {
				return
			}

			devices, err := h.manager.ListDevices()
			if err != nil {
				h.tracer.Error("SSE: falha ao listar dispositivos: %v", err)
				continue
			}

			payload := gin.H{
				"device_id": event.DeviceID,
				"status":    event.Status,
				"counts":    countFleet(devices),
			}

			// A linha inteira acompanha o evento, para o cliente atualizar
			// contadores de usuários e flags sem uma segunda requisição.
			for _, d := range devices {
				if d.ID == event.DeviceID {
					payload["device"] = newDeviceView(d)
					break
				}
			}

			if err := writeSSE(c.Writer, "device", payload); err != nil {
				return
			}
			flusher.Flush()

		case <-ticker.C:
			// Comentário SSE: mantém a conexão quente e é ignorado pelo
			// EventSource.
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-clientGone:
			return
		}
	}
}

// writeSnapshot manda a frota inteira, para o cliente partir de um estado
// conhecido em vez de confiar no que veio no HTML.
func (h *Handler) writeSnapshot(w io.Writer) error {
	devices, err := h.manager.ListDevices()
	if err != nil {
		return err
	}

	views := make([]deviceView, 0, len(devices))
	for _, d := range devices {
		views = append(views, newDeviceView(d))
	}

	return writeSSE(w, "snapshot", gin.H{
		"devices": views,
		"counts":  countFleet(devices),
	})
}
```

- [ ] **Step 4: Rodar e confirmar que passa**

Run: `go test ./internal/handlers/ -run TestWriteSSE -v`
Expected: PASS nos dois testes.

- [ ] **Step 5: Trocar a rota e remover o handler antigo**

Em `handlers.go`, trocar:

```go
	router.GET("/events", h.handleSSE)
```

por:

```go
	router.GET("/events", h.handleStream)
```

Apagar a função `handleSSE` inteira (hoje da linha 866 até o `}` antes de
`getPoolStats`).

- [ ] **Step 6: Verificar build e suíte**

Run: `go build ./... && go test ./... 2>&1 | tail -20`
Expected: build sem saída, testes PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/handlers
git commit -m "feat(ui): SSE com snapshot no connect, keepalive e retry"
```

---

### Task 4: Fundação visual — fontes, tokens e sprite de ícones

Nada consome esses arquivos ainda; as tarefas seguintes constroem sobre
eles. Separado porque baixar e verificar fontes é um passo que dá errado de
formas próprias, e misturá-lo com layout esconde a causa.

**Files:**
- Create: `assets/web/static/fonts/IBMPlexSans-{Regular,Medium,SemiBold}.woff2`
- Create: `assets/web/static/fonts/IBMPlexMono-{Regular,Medium}.woff2`
- Create: `assets/web/static/css/tokens.css`
- Create: `assets/web/static/css/base.css`
- Create: `assets/web/static/icons.svg`

**Interfaces:**
- Produces: custom properties em `:root` consumidas por todo CSS posterior
- Produces: símbolos SVG referenciados como `<svg class="icon"><use href="/static/icons.svg#play"></use></svg>`

- [ ] **Step 1: Baixar as fontes**

O CSS da API do Google só devolve `woff2` para User-Agents modernos; sem o
`-A`, vem `ttf`, que é três vezes maior.

```bash
cd "c:/Personal Development/Personal-Projects/GoFacialEmulator"
mkdir -p assets/web/static/fonts
UA='Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36'

for spec in "IBM+Plex+Sans:wght@400;500;600" "IBM+Plex+Mono:wght@400;500"; do
  curl -sS -A "$UA" "https://fonts.googleapis.com/css2?family=${spec}&display=swap"
done > /tmp/plex.css

grep -o 'https://fonts.gstatic.com[^)]*\.woff2' /tmp/plex.css | sort -u
```

Baixar cada URL listada e nomear conforme família e peso — a ordem no CSS
segue os pesos pedidos (400, 500, 600 para Sans; 400, 500 para Mono), e
cada bloco `@font-face` traz `font-family` e `font-weight` logo acima do
`src`, então a correspondência é lida do próprio arquivo.

Salvar como `IBMPlexSans-Regular.woff2`, `IBMPlexSans-Medium.woff2`,
`IBMPlexSans-SemiBold.woff2`, `IBMPlexMono-Regular.woff2`,
`IBMPlexMono-Medium.woff2`.

- [ ] **Step 2: Verificar os arquivos baixados**

```bash
ls -la assets/web/static/fonts/
file assets/web/static/fonts/*.woff2
```

Expected: cinco arquivos, cada um entre 20KB e 120KB, e `file` reportando
`Web Open Font Format (Version 2)`. Um arquivo de poucos bytes ou
identificado como HTML significa que o download trouxe uma página de erro.

- [ ] **Step 3: Escrever `assets/web/static/css/tokens.css`**

```css
/* ==========================================================================
   Tokens — Facial Emulators
   Fonte única de cor, tipografia e espaçamento. Nenhum outro arquivo
   declara um valor literal de cor.
   ========================================================================== */

@font-face {
  font-family: 'IBM Plex Sans';
  src: url('/static/fonts/IBMPlexSans-Regular.woff2') format('woff2');
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Sans';
  src: url('/static/fonts/IBMPlexSans-Medium.woff2') format('woff2');
  font-weight: 500;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Sans';
  src: url('/static/fonts/IBMPlexSans-SemiBold.woff2') format('woff2');
  font-weight: 600;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Mono';
  src: url('/static/fonts/IBMPlexMono-Regular.woff2') format('woff2');
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Mono';
  src: url('/static/fonts/IBMPlexMono-Medium.woff2') format('woff2');
  font-weight: 500;
  font-style: normal;
  font-display: swap;
}

:root {
  /* Superfícies, do fundo para o topo */
  --ink-900: #0e1116;
  --ink-800: #151a22;
  --ink-700: #1c222c;
  --ink-600: #262d3a;
  --ink-500: #333c4d;

  /* Texto */
  --text-hi: #e6ebf2;
  --text-mid: #9aa6ba;
  --text-low: #6b7688;

  /* Ação. Laranja Invenzi. Nunca comunica estado — só "você pode agir
     aqui" e foco de teclado. Estado é live/halt/idle abaixo. */
  --signal: #ff8c00;
  --signal-hi: #ffa733;
  --signal-dim: #40301a;

  /* Estado */
  --live: #3ddc97;
  --halt: #ff5c5c;
  --idle: #58637a;
  --warn: #ffc247;

  /* Fundos de estado, para badges e faixas de linha */
  --live-dim: #16302a;
  --halt-dim: #34191d;
  --idle-dim: #1f242e;
  --warn-dim: #322913;

  /* Tipografia */
  --font-ui: 'IBM Plex Sans', system-ui, -apple-system, 'Segoe UI', sans-serif;
  --font-mono: 'IBM Plex Mono', ui-monospace, 'Cascadia Mono', Consolas, monospace;

  --size-xs: 0.75rem;   /* 12px — rótulos de coluna, legendas */
  --size-sm: 0.8125rem; /* 13px — corpo denso, células */
  --size-md: 0.9375rem; /* 15px — corpo padrão, campos */
  --size-lg: 1.25rem;   /* 20px — título de página */
  --size-xl: 1.75rem;   /* 28px — números do medidor */

  /* Espaçamento — escala de 4px */
  --gap-1: 0.25rem;
  --gap-2: 0.5rem;
  --gap-3: 0.75rem;
  --gap-4: 1rem;
  --gap-5: 1.5rem;
  --gap-6: 2rem;

  --radius-sm: 3px;
  --radius-md: 5px;
  --radius-lg: 8px;

  --shadow-panel: 0 1px 2px rgb(0 0 0 / 40%);
  --shadow-float: 0 8px 24px rgb(0 0 0 / 50%);

  /* Dimensões do shell */
  --rail-collapsed: 64px;
  --rail-expanded: 216px;
  --topbar-height: 56px;
  --row-height: 40px;

  --ease: cubic-bezier(0.2, 0, 0.2, 1);
}
```

- [ ] **Step 4: Escrever `assets/web/static/css/base.css`**

```css
/* ==========================================================================
   Base — reset e elementos
   ========================================================================== */

*,
*::before,
*::after {
  box-sizing: border-box;
}

html {
  -webkit-text-size-adjust: 100%;
}

body {
  margin: 0;
  /* Pintado explicitamente: a aplicação assume tema escuro único, então
     nada pode herdar o fundo do agente do usuário. */
  background: var(--ink-900);
  color: var(--text-hi);
  font-family: var(--font-ui);
  font-size: var(--size-md);
  line-height: 1.5;
  -webkit-font-smoothing: antialiased;
}

h1, h2, h3 {
  margin: 0;
  font-weight: 600;
  line-height: 1.25;
  letter-spacing: -0.01em;
}

h1 { font-size: var(--size-lg); }
h2 { font-size: var(--size-md); }
h3 { font-size: var(--size-sm); }

p { margin: 0 0 var(--gap-3); }

a {
  color: var(--signal);
  text-decoration: none;
}

a:hover { color: var(--signal-hi); }

/* Um único anel de foco para toda a aplicação. Laranja porque foco é
   affordance de ação, e o laranja está reservado para isso. */
:focus-visible {
  outline: 2px solid var(--signal);
  outline-offset: 2px;
  border-radius: var(--radius-sm);
}

:focus:not(:focus-visible) { outline: none; }

/* Números em tabela precisam alinhar coluna a coluna. */
.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
  font-size: var(--size-sm);
}

.label {
  font-size: var(--size-xs);
  font-weight: 500;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--text-mid);
}

.icon {
  width: 1em;
  height: 1em;
  fill: currentColor;
  flex: none;
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
```

- [ ] **Step 5: Escrever `assets/web/static/icons.svg`**

Substitui Bootstrap Icons e Font Awesome. Um arquivo, cacheável, sem
requisição externa.

```xml
<svg xmlns="http://www.w3.org/2000/svg" style="display:none">
  <symbol id="play" viewBox="0 0 16 16"><path d="M4 2.5v11l9-5.5-9-5.5z"/></symbol>
  <symbol id="stop" viewBox="0 0 16 16"><rect x="4" y="4" width="8" height="8" rx="1"/></symbol>
  <symbol id="devices" viewBox="0 0 16 16"><path d="M3 1h10a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V2a1 1 0 0 1 1-1zm0 1.5v11h10v-11H3zM6.5 12h3v1h-3v-1z"/></symbol>
  <symbol id="compare" viewBox="0 0 16 16"><path d="M6.5 3.5 3 7l3.5 3.5V8H13V6H6.5V3.5zM9.5 12.5 13 9l-3.5-3.5V8H3v2h6.5v2.5z"/></symbol>
  <symbol id="refresh" viewBox="0 0 16 16"><path d="M8 3a5 5 0 1 0 4.9 6h-1.6A3.4 3.4 0 1 1 8 4.6c.9 0 1.7.4 2.3 1L8.5 7.4H13V3l-1.5 1.5A5 5 0 0 0 8 3z"/></symbol>
  <symbol id="gear" viewBox="0 0 16 16"><path d="M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zm0 1.5a1 1 0 1 1 0 2 1 1 0 0 1 0-2z"/><path d="m6.9 1 .3 1.6a5.5 5.5 0 0 0-1 .6L4.7 2.6 3.3 5l1.2 1a5.5 5.5 0 0 0 0 1.2l-1.2 1 1.4 2.4 1.5-.6c.3.2.6.4 1 .6L6.9 15h2.2l.3-1.6c.4-.2.7-.4 1-.6l1.5.6 1.4-2.4-1.2-1a5.5 5.5 0 0 0 0-1.2l1.2-1L11.9 2.6l-1.5.6a5.5 5.5 0 0 0-1-.6L9.1 1H6.9z" opacity=".85"/></symbol>
  <symbol id="menu" viewBox="0 0 16 16"><path d="M2 4h12v1.5H2V4zm0 3.25h12v1.5H2v-1.5zM2 11h12v1.5H2V11z"/></symbol>
  <symbol id="search" viewBox="0 0 16 16"><path d="M7 2a5 5 0 0 1 3.9 8.1l3 3-1 1-3-3A5 5 0 1 1 7 2zm0 1.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7z"/></symbol>
  <symbol id="close" viewBox="0 0 16 16"><path d="m4.3 3.3 3.7 3.7 3.7-3.7 1 1L9 8l3.7 3.7-1 1L8 9l-3.7 3.7-1-1L7 8 3.3 4.3l1-1z"/></symbol>
  <symbol id="check" viewBox="0 0 16 16"><path d="m13.3 4.3-7 7-3.6-3.6 1-1L6.3 9.3l6-6 1 1z"/></symbol>
  <symbol id="alert" viewBox="0 0 16 16"><path d="M8 1.5 15 14H1L8 1.5zm0 3L3.6 12.5h8.8L8 4.5zM7.25 7h1.5v3h-1.5V7zm0 3.75h1.5v1.25h-1.5v-1.25z"/></symbol>
  <symbol id="info" viewBox="0 0 16 16"><path d="M8 1.5a6.5 6.5 0 1 1 0 13 6.5 6.5 0 0 1 0-13zm0 1.5a5 5 0 1 0 0 10A5 5 0 0 0 8 3zm-.75 3.75h1.5v.75h-1.5v-.75zm0 1.75h1.5v3.5h-1.5V8.5z"/></symbol>
  <symbol id="chevron-left" viewBox="0 0 16 16"><path d="M10.3 3.3 5.6 8l4.7 4.7-1 1L3.6 8l5.7-5.7 1 1z"/></symbol>
  <symbol id="chevron-right" viewBox="0 0 16 16"><path d="m5.7 3.3 5.7 4.7-5.7 5.7-1-1L9.4 8 4.7 4.3l1-1z"/></symbol>
  <symbol id="users" viewBox="0 0 16 16"><path d="M6 2.5a2.75 2.75 0 1 1 0 5.5 2.75 2.75 0 0 1 0-5.5zm0 1.5a1.25 1.25 0 1 0 0 2.5A1.25 1.25 0 0 0 6 4zM6 9c2.5 0 4.5 1.2 4.5 2.75V13.5h-9v-1.75C1.5 10.2 3.5 9 6 9zm5.5-6.5a2.25 2.25 0 1 1 0 4.5 2.25 2.25 0 0 1 0-4.5zM12 9c1.4.3 2.5 1.3 2.5 2.5v2h-2.75v-1.75c0-1-.4-1.9-1.1-2.6.45-.1.9-.15 1.35-.15z"/></symbol>
</svg>
```

- [ ] **Step 6: Verificar que as fontes são servidas**

O `go:embed` já cobre `all:web`, então nada muda no Go. Confirmar que os
arquivos entraram no binário:

```bash
go build -o /tmp/emu-check ./cmd/emulator-service && echo "build ok"
grep -c "IBMPlex" <<< "$(ls assets/web/static/fonts/)"
```

Expected: `build ok` e contagem 5.

- [ ] **Step 7: Commit**

```bash
git add assets/web/static/fonts assets/web/static/css/tokens.css assets/web/static/css/base.css assets/web/static/icons.svg
git commit -m "feat(ui): fundação visual — IBM Plex embedado, tokens e sprite de ícones"
```

---

### Task 5: FleetStream e toasts

Uma conexão SSE por aba, com reconexão que não apaga os dados, e
notificações que substituem `alert()`.

**Files:**
- Create: `assets/web/static/js/realtime.js`
- Create: `assets/web/static/js/toast.js`

**Interfaces:**
- Consumes: `/events` com eventos `snapshot` e `device` (Task 3)
- Produces: `window.FleetStream` — `.subscribe(evento, fn) -> unsubscribe`, `.state` (`'live' | 'reconnecting' | 'down'`), `.counts`, `.start()`
- Produces: `window.Toast` — `.ok(msg)`, `.err(msg)`, `.info(msg)`

- [ ] **Step 1: Escrever `assets/web/static/js/toast.js`**

```js
/**
 * Toast — notificações não bloqueantes.
 *
 * Substitui os alert() espalhados pela aplicação. alert() trava a thread,
 * exige um clique para sumir e não empilha: iniciar oito emuladores dava
 * oito diálogos em fila.
 */
(function () {
  'use strict';

  var DURACAO = { ok: 3000, info: 4000, err: 6000 };

  function container() {
    var el = document.getElementById('toast-region');
    if (!el) {
      el = document.createElement('div');
      el.id = 'toast-region';
      el.className = 'toast-region';
      // polite, não assertive: são confirmações de ação, não emergências.
      el.setAttribute('role', 'status');
      el.setAttribute('aria-live', 'polite');
      document.body.appendChild(el);
    }
    return el;
  }

  function mostrar(tipo, mensagem) {
    var toast = document.createElement('div');
    toast.className = 'toast toast--' + tipo;

    var icone = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    icone.setAttribute('class', 'icon');
    var use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
    use.setAttribute('href', '/static/icons.svg#' + (tipo === 'err' ? 'alert' : tipo === 'ok' ? 'check' : 'info'));
    icone.appendChild(use);

    var texto = document.createElement('span');
    // textContent, não innerHTML: a mensagem pode carregar texto de erro
    // vindo do servidor.
    texto.textContent = mensagem;

    toast.appendChild(icone);
    toast.appendChild(texto);
    container().appendChild(toast);

    window.setTimeout(function () {
      toast.classList.add('toast--saindo');
      window.setTimeout(function () { toast.remove(); }, 200);
    }, DURACAO[tipo] || 4000);
  }

  window.Toast = {
    ok: function (msg) { mostrar('ok', msg); },
    err: function (msg) { mostrar('err', msg); },
    info: function (msg) { mostrar('info', msg); }
  };
})();
```

- [ ] **Step 2: Escrever `assets/web/static/js/realtime.js`**

```js
/**
 * FleetStream — conexão única com /events.
 *
 * A versão anterior abria um EventSource em header.js e outro no script
 * inline de devices.html: duas conexões por aba, dois listeners no manager
 * e dois caminhos de atualização que discordavam entre si. Aqui existe uma
 * conexão e um barramento de assinaturas.
 *
 * Eventos publicados:
 *   'snapshot' -> { devices: [...], counts: {...} }   estado completo
 *   'device'   -> { device_id, status, device, counts } uma mudança
 *   'status'   -> 'live' | 'reconnecting' | 'down'      saúde do stream
 */
(function () {
  'use strict';

  var BACKOFF_INICIAL = 1000;
  var BACKOFF_MAXIMO = 30000;
  // Só depois desse tempo sem stream é que entra o polling de resgate.
  var LIMIAR_FALLBACK = 30000;
  var INTERVALO_FALLBACK = 10000;

  var fonte = null;
  var backoff = BACKOFF_INICIAL;
  var timerReconexao = null;
  var timerFallback = null;
  var caiuEm = null;
  var assinantes = { snapshot: [], device: [], status: [] };

  var api = {
    state: 'down',
    counts: { total: 0, running: 0, stopped: 0, disabled: 0 },

    subscribe: function (evento, fn) {
      if (!assinantes[evento]) { return function () {}; }
      assinantes[evento].push(fn);
      return function () {
        var i = assinantes[evento].indexOf(fn);
        if (i !== -1) { assinantes[evento].splice(i, 1); }
      };
    },

    start: function () {
      if (fonte) { return; }
      conectar();
    }
  };

  function publicar(evento, dados) {
    assinantes[evento].slice().forEach(function (fn) {
      try {
        fn(dados);
      } catch (erro) {
        console.error('FleetStream: assinante de "' + evento + '" falhou', erro);
      }
    });
  }

  function definirEstado(novo) {
    if (api.state === novo) { return; }
    api.state = novo;
    publicar('status', novo);
  }

  function conectar() {
    fonte = new EventSource('/events');

    fonte.addEventListener('open', function () {
      backoff = BACKOFF_INICIAL;
      caiuEm = null;
      pararFallback();
      definirEstado('live');
    });

    fonte.addEventListener('snapshot', function (evento) {
      var dados = analisar(evento.data);
      if (!dados) { return; }
      api.counts = dados.counts;
      definirEstado('live');
      publicar('snapshot', dados);
    });

    fonte.addEventListener('device', function (evento) {
      var dados = analisar(evento.data);
      if (!dados) { return; }
      api.counts = dados.counts;
      publicar('device', dados);
    });

    fonte.addEventListener('error', function () {
      // O EventSource reconecta sozinho enquanto readyState é CONNECTING.
      // Só tratamos como queda quando ele desiste de vez.
      if (fonte.readyState !== EventSource.CLOSED) {
        definirEstado('reconnecting');
        return;
      }

      fonte.close();
      fonte = null;

      if (caiuEm === null) { caiuEm = Date.now(); }
      definirEstado('reconnecting');
      agendarFallback();

      timerReconexao = window.setTimeout(conectar, backoff);
      backoff = Math.min(backoff * 2, BACKOFF_MAXIMO);
    });
  }

  function analisar(bruto) {
    try {
      return JSON.parse(bruto);
    } catch (erro) {
      console.error('FleetStream: payload inválido', erro, bruto);
      return null;
    }
  }

  /**
   * Rede de segurança: se o stream não voltar em LIMIAR_FALLBACK, passa a
   * consultar /api/status. Note que ele nunca sobrescreve os dados com
   * zeros — o bug antigo pintava "Offline" e apagava as contagens reais na
   * primeira falha de conexão.
   */
  function agendarFallback() {
    if (timerFallback) { return; }

    timerFallback = window.setInterval(function () {
      if (caiuEm === null || Date.now() - caiuEm < LIMIAR_FALLBACK) { return; }

      definirEstado('down');

      fetch('/api/status')
        .then(function (r) { return r.ok ? r.json() : Promise.reject(r.status); })
        .then(function (dados) {
          api.counts = {
            total: dados.total_devices,
            running: dados.running_devices,
            stopped: dados.stopped_devices,
            disabled: dados.disabled_devices
          };
          publicar('device', { counts: api.counts });
        })
        .catch(function () { /* segue tentando no próximo tick */ });
    }, INTERVALO_FALLBACK);
  }

  function pararFallback() {
    if (timerFallback) {
      window.clearInterval(timerFallback);
      timerFallback = null;
    }
  }

  window.addEventListener('beforeunload', function () {
    if (timerReconexao) { window.clearTimeout(timerReconexao); }
    pararFallback();
    if (fonte) { fonte.close(); }
  });

  window.FleetStream = api;
})();
```

- [ ] **Step 3: Verificar sintaxe**

```bash
node --check assets/web/static/js/realtime.js && node --check assets/web/static/js/toast.js && echo "sintaxe ok"
```

Expected: `sintaxe ok`. Se `node` não estiver disponível, abrir os dois
arquivos no browser via `<script>` e confirmar console limpo — a
verificação real vem na Task 7.

- [ ] **Step 4: Commit**

```bash
git add assets/web/static/js/realtime.js assets/web/static/js/toast.js
git commit -m "feat(ui): FleetStream com conexão SSE única e toasts não bloqueantes"
```

---

### Task 6: Shell do console — topbar com fleet meter, rail e componentes

Aqui a aplicação muda de cara e o CDN sai. É a maior tarefa do plano
porque topbar, rail e a folha de componentes formam uma unidade: entregar
`base.html` sem eles deixaria a aplicação sem estilo nenhum entre commits.

**Files:**
- Create: `assets/web/static/css/layout.css`
- Create: `assets/web/static/css/components.css`
- Create: `assets/web/static/js/app.js`
- Modify: `assets/web/templates/base.html`
- Modify: `assets/web/templates/header.html`
- Modify: `assets/web/templates/sidebar.html`
- Modify: `assets/web/templates/footer.html`

**Interfaces:**
- Consumes: tokens de `tokens.css` (Task 4), `window.FleetStream` e `window.Toast` (Task 5)
- Produces: classes `.shell`, `.topbar`, `.rail`, `.content`, `.meter`, `.btn`, `.field`, `.grid`, `.badge`, `.led`, `.drawer`, `.toast`
- Produces: `window.FleetMeter.render(counts)` — chamado por `app.js` a cada evento

- [ ] **Step 1: Escrever `assets/web/static/css/layout.css`**

```css
/* ==========================================================================
   Layout — shell da aplicação
   ========================================================================== */

.shell {
  display: grid;
  grid-template-columns: var(--rail-collapsed) 1fr;
  grid-template-rows: var(--topbar-height) 1fr;
  grid-template-areas:
    'topbar topbar'
    'rail   content';
  min-height: 100vh;
  transition: grid-template-columns 0.2s var(--ease);
}

.shell[data-rail='expanded'] {
  grid-template-columns: var(--rail-expanded) 1fr;
}

/* --------------------------------------------------------------------------
   Topbar
   -------------------------------------------------------------------------- */

.topbar {
  grid-area: topbar;
  position: sticky;
  top: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  gap: var(--gap-5);
  padding: 0 var(--gap-4);
  background: var(--ink-700);
  border-bottom: 1px solid var(--ink-600);
}

.topbar__brand {
  display: flex;
  align-items: center;
  gap: var(--gap-2);
  color: var(--text-hi);
  font-weight: 600;
  letter-spacing: -0.01em;
  white-space: nowrap;
}

.topbar__brand:hover { color: var(--text-hi); }

.topbar__mark {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  border-radius: var(--radius-sm);
  background: var(--signal);
  color: var(--ink-900);
}

.topbar__version {
  font-family: var(--font-mono);
  font-size: var(--size-xs);
  color: var(--text-low);
}

.topbar__spacer { flex: 1; }

.topbar__logo {
  max-height: 26px;
  max-width: 96px;
  object-fit: contain;
}

.topbar__logo-fallback {
  font-size: var(--size-xs);
  font-weight: 600;
  letter-spacing: 0.08em;
  color: var(--text-mid);
}

/* --------------------------------------------------------------------------
   Rail
   -------------------------------------------------------------------------- */

.rail {
  grid-area: rail;
  position: sticky;
  top: var(--topbar-height);
  align-self: start;
  height: calc(100vh - var(--topbar-height));
  display: flex;
  flex-direction: column;
  gap: var(--gap-1);
  padding: var(--gap-3) var(--gap-2);
  background: var(--ink-800);
  border-right: 1px solid var(--ink-600);
  overflow: hidden;
}

.rail__item {
  display: flex;
  align-items: center;
  gap: var(--gap-3);
  padding: var(--gap-2) var(--gap-3);
  border: 0;
  border-left: 2px solid transparent;
  border-radius: var(--radius-md);
  background: none;
  color: var(--text-mid);
  font: inherit;
  font-size: var(--size-sm);
  text-align: left;
  white-space: nowrap;
  cursor: pointer;
  transition: background 0.15s var(--ease), color 0.15s var(--ease);
}

.rail__item .icon {
  width: 18px;
  height: 18px;
}

.rail__item:hover {
  background: var(--ink-700);
  color: var(--text-hi);
}

/* Item ativo: barra laranja na borda. É a única marcação permanente de
   laranja na navegação — o resto do rail fica neutro. */
.rail__item[aria-current='page'] {
  background: var(--ink-700);
  border-left-color: var(--signal);
  color: var(--text-hi);
}

.rail__item--danger:hover { color: var(--halt); }
.rail__item--go:hover { color: var(--live); }

.rail__label {
  opacity: 1;
  transition: opacity 0.15s var(--ease);
}

.shell:not([data-rail='expanded']) .rail__label {
  opacity: 0;
  pointer-events: none;
}

.rail__divider {
  height: 1px;
  margin: var(--gap-2) var(--gap-2);
  background: var(--ink-600);
}

/* --------------------------------------------------------------------------
   Conteúdo
   -------------------------------------------------------------------------- */

.content {
  grid-area: content;
  min-width: 0; /* deixa a tabela rolar sem estourar o grid */
  padding: var(--gap-5) var(--gap-5) var(--gap-6);
}

.page-head {
  display: flex;
  align-items: center;
  gap: var(--gap-4);
  margin-bottom: var(--gap-4);
}

.page-head__actions {
  margin-left: auto;
  display: flex;
  gap: var(--gap-2);
}

.footer {
  grid-column: 1 / -1;
  padding: var(--gap-4);
  color: var(--text-low);
  font-size: var(--size-xs);
  text-align: center;
}

.page-error {
  max-width: 34rem;
  margin: var(--gap-6) auto;
  padding: var(--gap-5);
  background: var(--ink-800);
  border: 1px solid var(--ink-600);
  border-left: 3px solid var(--halt);
  border-radius: var(--radius-lg);
}

.page-error__title { margin-bottom: var(--gap-3); }

.page-error__detail {
  font-family: var(--font-mono);
  font-size: var(--size-sm);
  color: var(--halt);
  word-break: break-word;
}

.page-error__hint { color: var(--text-mid); }

@media (max-width: 720px) {
  .shell,
  .shell[data-rail='expanded'] {
    grid-template-columns: var(--rail-collapsed) 1fr;
  }

  .content { padding: var(--gap-4) var(--gap-3) var(--gap-5); }
}
```

- [ ] **Step 2: Escrever `assets/web/static/css/components.css`**

```css
/* ==========================================================================
   Componentes
   ========================================================================== */

/* --------------------------------------------------------------------------
   Fleet meter — elemento assinatura.
   Um segmento por emulador, colorido por estado: a frota inteira legível
   sem ler número nenhum. O mesmo vocabulário de cor reaparece no LED de
   cada linha da tabela.
   -------------------------------------------------------------------------- */

.meter {
  display: flex;
  align-items: center;
  gap: var(--gap-3);
}

.meter__bar {
  display: flex;
  gap: 2px;
  align-items: stretch;
  height: 20px;
  min-width: 120px;
  padding: 3px;
  background: var(--ink-800);
  border: 1px solid var(--ink-600);
  border-radius: var(--radius-sm);
}

.meter__seg {
  flex: 1 1 3px;
  min-width: 2px;
  max-width: 6px;
  border-radius: 1px;
  background: var(--idle);
  transition: background 0.3s var(--ease);
}

.meter__seg[data-state='running'] { background: var(--live); }
.meter__seg[data-state='stopped'] { background: var(--halt); }
.meter__seg[data-state='disabled'] { background: var(--ink-600); }

.meter__reading {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
  font-size: var(--size-sm);
  color: var(--text-mid);
  white-space: nowrap;
}

.meter__reading b {
  color: var(--live);
  font-weight: 500;
}

/* Saúde do stream. 'live' não mostra nada: silêncio é o estado normal e
   um selo verde permanente vira ruído. */
.meter__health {
  font-size: var(--size-xs);
  color: var(--warn);
  white-space: nowrap;
}

.meter__health:empty { display: none; }

/* --------------------------------------------------------------------------
   LED de estado
   -------------------------------------------------------------------------- */

.led {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--idle);
  vertical-align: middle;
}

.led[data-state='running'] {
  background: var(--live);
  box-shadow: 0 0 6px color-mix(in srgb, var(--live) 55%, transparent);
}

.led[data-state='stopped'] { background: var(--halt); }
.led[data-state='disabled'] { background: var(--ink-500); }

/* Flash de transição: a mudança em tempo real precisa ser percebida, não
   só refletida. */
@keyframes led-flash {
  0% { transform: scale(1); }
  35% { transform: scale(2.1); }
  100% { transform: scale(1); }
}

.led--flash { animation: led-flash 0.5s var(--ease); }

@media (prefers-reduced-motion: reduce) {
  .led--flash { animation: none; }
}

/* --------------------------------------------------------------------------
   Botões
   -------------------------------------------------------------------------- */

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--gap-2);
  height: 32px;
  padding: 0 var(--gap-3);
  border: 1px solid var(--ink-500);
  border-radius: var(--radius-md);
  background: var(--ink-700);
  color: var(--text-hi);
  font: inherit;
  font-size: var(--size-sm);
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  transition: background 0.15s var(--ease), border-color 0.15s var(--ease);
}

.btn:hover:not(:disabled) {
  background: var(--ink-600);
  border-color: var(--ink-500);
}

.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn--action {
  background: var(--signal-dim);
  border-color: var(--signal);
  color: var(--signal);
}

.btn--action:hover:not(:disabled) {
  background: var(--signal);
  color: var(--ink-900);
}

.btn--go:hover:not(:disabled) {
  border-color: var(--live);
  color: var(--live);
}

.btn--halt:hover:not(:disabled) {
  border-color: var(--halt);
  color: var(--halt);
}

.btn--icon {
  width: 32px;
  padding: 0;
}

.btn--sm {
  height: 26px;
  padding: 0 var(--gap-2);
}

/* Estado pendente: a ação foi enviada e ainda não voltou. Antes, start e
   stop eram fire-and-forget e a linha ficava idêntica até o SSE chegar. */
.btn[data-pending='true'] {
  opacity: 0.5;
  pointer-events: none;
}

/* --------------------------------------------------------------------------
   Campos
   -------------------------------------------------------------------------- */

.field {
  display: flex;
  flex-direction: column;
  gap: var(--gap-1);
  min-width: 0;
}

.field__label {
  font-size: var(--size-xs);
  font-weight: 500;
  color: var(--text-mid);
}

.input,
.select {
  height: 32px;
  padding: 0 var(--gap-2);
  border: 1px solid var(--ink-500);
  border-radius: var(--radius-md);
  background: var(--ink-900);
  color: var(--text-hi);
  font: inherit;
  font-size: var(--size-sm);
}

.input::placeholder { color: var(--text-low); }

.input:focus,
.select:focus {
  border-color: var(--signal);
  outline: none;
  box-shadow: 0 0 0 2px var(--signal-dim);
}

.input--mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--gap-3);
  padding: var(--gap-3);
  margin-bottom: var(--gap-4);
  background: var(--ink-800);
  border: 1px solid var(--ink-600);
  border-radius: var(--radius-lg);
}

.filters .field { flex: 1 1 10rem; }

.filters__actions {
  display: flex;
  gap: var(--gap-2);
}

/* --------------------------------------------------------------------------
   Tabela
   -------------------------------------------------------------------------- */

.table-wrap {
  overflow-x: auto;
  background: var(--ink-800);
  border: 1px solid var(--ink-600);
  border-radius: var(--radius-lg);
}

.grid {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--size-sm);
}

.grid thead th {
  position: sticky;
  top: var(--topbar-height);
  z-index: 10;
  padding: var(--gap-2) var(--gap-3);
  background: var(--ink-700);
  border-bottom: 1px solid var(--ink-600);
  font-size: var(--size-xs);
  font-weight: 500;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--text-mid);
  text-align: left;
  white-space: nowrap;
}

.grid td {
  height: var(--row-height);
  padding: 0 var(--gap-3);
  border-bottom: 1px solid var(--ink-700);
  color: var(--text-hi);
  white-space: nowrap;
}

.grid tbody tr:last-child td { border-bottom: 0; }

.grid tbody tr:hover td { background: var(--ink-700); }

/* Números alinham à direita para as ordens de grandeza serem comparáveis
   de relance; nome e modelo ficam à esquerda, que é como se lê texto. */
.grid .num {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.grid .center { text-align: center; }

.grid__id {
  display: inline-flex;
  align-items: center;
  gap: var(--gap-2);
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.grid__name {
  max-width: 20rem;
  overflow: hidden;
  text-overflow: ellipsis;
}

.grid tr[data-state='disabled'] td { color: var(--text-low); }

.row-actions {
  display: flex;
  gap: var(--gap-1);
}

/* --------------------------------------------------------------------------
   Badge de estado
   -------------------------------------------------------------------------- */

.badge {
  display: inline-flex;
  align-items: center;
  gap: var(--gap-1);
  padding: 2px var(--gap-2);
  border-radius: 999px;
  font-size: var(--size-xs);
  font-weight: 500;
  background: var(--idle-dim);
  color: var(--text-mid);
}

.badge[data-state='running'] { background: var(--live-dim); color: var(--live); }
.badge[data-state='stopped'] { background: var(--halt-dim); color: var(--halt); }
.badge[data-state='disabled'] { background: var(--idle-dim); color: var(--text-low); }
.badge[data-state='error'] { background: var(--warn-dim); color: var(--warn); }

/* --------------------------------------------------------------------------
   Empty state
   -------------------------------------------------------------------------- */

.empty {
  padding: var(--gap-6) var(--gap-4);
  text-align: center;
  color: var(--text-mid);
}

.empty__title {
  margin-bottom: var(--gap-2);
  color: var(--text-hi);
  font-weight: 500;
}

/* --------------------------------------------------------------------------
   Paginação
   -------------------------------------------------------------------------- */

.pager {
  display: flex;
  align-items: center;
  gap: var(--gap-3);
  margin-top: var(--gap-4);
}

.pager__pages {
  display: flex;
  gap: var(--gap-1);
  margin-left: auto;
}

.pager__page {
  display: grid;
  place-items: center;
  min-width: 30px;
  height: 30px;
  padding: 0 var(--gap-1);
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  color: var(--text-mid);
  font-family: var(--font-mono);
  font-size: var(--size-sm);
}

.pager__page:hover {
  background: var(--ink-700);
  color: var(--text-hi);
}

.pager__page[aria-current='page'] {
  border-color: var(--signal);
  color: var(--signal);
}

.pager__page[aria-disabled='true'] {
  color: var(--text-low);
  pointer-events: none;
}

/* --------------------------------------------------------------------------
   Drawer
   -------------------------------------------------------------------------- */

.drawer {
  position: fixed;
  inset: 0 0 0 auto;
  z-index: 40;
  width: min(560px, 100vw);
  display: flex;
  flex-direction: column;
  background: var(--ink-800);
  border-left: 1px solid var(--ink-600);
  box-shadow: var(--shadow-float);
  transform: translateX(100%);
  transition: transform 0.2s var(--ease);
}

.drawer[data-open='true'] { transform: translateX(0); }

.drawer__head {
  display: flex;
  align-items: center;
  gap: var(--gap-3);
  padding: var(--gap-3) var(--gap-4);
  border-bottom: 1px solid var(--ink-600);
}

.drawer__title {
  display: flex;
  align-items: center;
  gap: var(--gap-2);
  font-weight: 600;
}

.drawer__close { margin-left: auto; }

.drawer__tabs {
  display: flex;
  gap: var(--gap-1);
  padding: 0 var(--gap-4);
  border-bottom: 1px solid var(--ink-600);
}

.drawer__tab {
  padding: var(--gap-2) var(--gap-3);
  border: 0;
  border-bottom: 2px solid transparent;
  background: none;
  color: var(--text-mid);
  font: inherit;
  font-size: var(--size-sm);
  cursor: pointer;
}

.drawer__tab[aria-selected='true'] {
  border-bottom-color: var(--signal);
  color: var(--text-hi);
}

.drawer__body {
  flex: 1;
  padding: var(--gap-4);
  overflow-y: auto;
}

.drawer-scrim {
  position: fixed;
  inset: 0;
  z-index: 35;
  background: rgb(0 0 0 / 55%);
}

.drawer-scrim[hidden] { display: none; }

/* --------------------------------------------------------------------------
   Cards de formulário
   -------------------------------------------------------------------------- */

.panel {
  max-width: 34rem;
  margin-bottom: var(--gap-5);
  background: var(--ink-800);
  border: 1px solid var(--ink-600);
  border-radius: var(--radius-lg);
}

.panel__head {
  display: flex;
  align-items: center;
  gap: var(--gap-2);
  padding: var(--gap-3) var(--gap-4);
  border-bottom: 1px solid var(--ink-600);
  font-weight: 600;
  font-size: var(--size-sm);
}

.panel__body {
  display: flex;
  flex-direction: column;
  gap: var(--gap-3);
  padding: var(--gap-4);
}

.panel__note {
  color: var(--text-mid);
  font-size: var(--size-sm);
}

.panel__note code {
  font-family: var(--font-mono);
  font-size: var(--size-xs);
  color: var(--signal);
}

.input-group {
  display: flex;
  gap: var(--gap-1);
}

.input-group .input { flex: 1; }

/* --------------------------------------------------------------------------
   Toasts
   -------------------------------------------------------------------------- */

.toast-region {
  position: fixed;
  right: var(--gap-4);
  bottom: var(--gap-4);
  z-index: 50;
  display: flex;
  flex-direction: column;
  gap: var(--gap-2);
  pointer-events: none;
}

.toast {
  display: flex;
  align-items: center;
  gap: var(--gap-2);
  max-width: 24rem;
  padding: var(--gap-2) var(--gap-3);
  background: var(--ink-700);
  border: 1px solid var(--ink-500);
  border-left: 3px solid var(--text-mid);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-float);
  font-size: var(--size-sm);
  animation: toast-in 0.2s var(--ease);
}

.toast--ok { border-left-color: var(--live); }
.toast--ok .icon { color: var(--live); }
.toast--err { border-left-color: var(--halt); }
.toast--err .icon { color: var(--halt); }
.toast--info { border-left-color: var(--signal); }
.toast--info .icon { color: var(--signal); }

.toast--saindo {
  opacity: 0;
  transition: opacity 0.2s var(--ease);
}

@keyframes toast-in {
  from { opacity: 0; transform: translateY(6px); }
  to { opacity: 1; transform: translateY(0); }
}

/* --------------------------------------------------------------------------
   Checkbox
   -------------------------------------------------------------------------- */

.check {
  width: 15px;
  height: 15px;
  accent-color: var(--signal);
  cursor: pointer;
}

.check:disabled { cursor: not-allowed; opacity: 0.4; }
```

- [ ] **Step 3: Reescrever `assets/web/templates/base.html`**

```html
<!DOCTYPE html>
<html lang="pt-br">

<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="color-scheme" content="dark">
  <title>{{ block "title" . }}Facial Emulators{{ end }}</title>

  <link rel="stylesheet" href="/static/css/tokens.css">
  <link rel="stylesheet" href="/static/css/base.css">
  <link rel="stylesheet" href="/static/css/layout.css">
  <link rel="stylesheet" href="/static/css/components.css">

  {{ block "additional_css" . }}{{ end }}
</head>

<body>
  <div class="shell" id="shell" data-rail="collapsed">
    {{ template "header.html" . }}
    {{ template "sidebar.html" . }}

    <main class="content">
      {{ block "content" . }}{{ end }}
    </main>

    {{ template "footer.html" . }}
  </div>

  <script src="/static/js/toast.js"></script>
  <script src="/static/js/realtime.js"></script>
  <script src="/static/js/app.js"></script>

  {{ block "additional_js" . }}{{ end }}
</body>

</html>
```

- [ ] **Step 4: Reescrever `assets/web/templates/header.html`**

```html
<header class="topbar">
  <a class="topbar__brand" href="/">
    <span class="topbar__mark">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#devices"></use></svg>
    </span>
    Facial Emulators
    {{ if .app_version }}
    <span class="topbar__version">v{{ .app_version }}</span>
    {{ end }}
  </a>

  <!-- Fleet meter: um segmento por emulador. As contagens do servidor
       semeiam o estado inicial; o FleetStream assume a partir do snapshot. -->
  <div class="meter" id="fleet-meter"
       data-total="{{ .counter_cards.total | default 0 }}"
       data-running="{{ .counter_cards.running | default 0 }}"
       data-stopped="{{ .counter_cards.stopped | default 0 }}"
       data-disabled="{{ .counter_cards.disabled | default 0 }}">
    <div class="meter__bar" id="meter-bar" role="img"
         aria-label="Estado da frota de emuladores"></div>
    <span class="meter__reading" id="meter-reading"></span>
    <span class="meter__health" id="meter-health"></span>
  </div>

  <span class="topbar__spacer"></span>

  <a href="/settings" class="btn btn--icon" title="Configurações" aria-label="Configurações">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#gear"></use></svg>
  </a>

  <img src="/static/images/logo.png" alt="Invenzi" class="topbar__logo" id="brand-logo">
  <span class="topbar__logo-fallback" id="brand-logo-fallback" hidden>INVENZI</span>
</header>
```

- [ ] **Step 5: Reescrever `assets/web/templates/sidebar.html`**

Sem `<script>` e sem `<style>`: o comportamento vai para `app.js`, o
visual para `layout.css`.

```html
<nav class="rail" id="rail" aria-label="Navegação principal">
  <button type="button" class="rail__item" id="rail-toggle"
          aria-expanded="false" aria-controls="rail">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#menu"></use></svg>
    <span class="rail__label">Recolher</span>
  </button>

  <div class="rail__divider"></div>

  <a href="/" class="rail__item" data-nav="/">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#devices"></use></svg>
    <span class="rail__label">Dispositivos</span>
  </a>

  <a href="/comparison" class="rail__item" data-nav="/comparison">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#compare"></use></svg>
    <span class="rail__label">Comparação</span>
  </a>

  <div class="rail__divider"></div>

  <button type="button" class="rail__item rail__item--go" id="start-all">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#play"></use></svg>
    <span class="rail__label">Iniciar todos</span>
  </button>

  <button type="button" class="rail__item rail__item--danger" id="stop-all">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#stop"></use></svg>
    <span class="rail__label">Parar todos</span>
  </button>

  <button type="button" class="rail__item" id="sync-db">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#refresh"></use></svg>
    <span class="rail__label">Sincronizar W-Access</span>
  </button>
</nav>
```

- [ ] **Step 6: Reescrever `assets/web/templates/footer.html`**

O ano fixo "© 2024" sai; a versão do app é a informação que de fato ajuda
quem reporta um problema.

```html
<footer class="footer">
  Invenzi · Facial Emulators{{ if .app_version }} v{{ .app_version }}{{ end }}
</footer>
```

- [ ] **Step 7: Escrever `assets/web/static/js/app.js`**

```js
/**
 * app.js — comportamento do shell: rail, fleet meter, ações globais.
 */
(function () {
  'use strict';

  var CHAVE_RAIL = 'fe.rail.expanded';

  // ------------------------------------------------------------------
  // Rail
  // ------------------------------------------------------------------

  function iniciarRail() {
    var shell = document.getElementById('shell');
    var toggle = document.getElementById('rail-toggle');
    if (!shell || !toggle) { return; }

    // O estado antes era perdido a cada navegação, e o menu voltava
    // recolhido em toda página.
    var expandido = window.localStorage.getItem(CHAVE_RAIL) === 'true';
    aplicar(expandido);

    toggle.addEventListener('click', function () {
      expandido = !expandido;
      aplicar(expandido);
      window.localStorage.setItem(CHAVE_RAIL, String(expandido));
    });

    function aplicar(aberto) {
      // Classe/atributo, não escrita de estilo em JS: a transição fica no
      // CSS e o prefers-reduced-motion continua valendo.
      shell.setAttribute('data-rail', aberto ? 'expanded' : 'collapsed');
      toggle.setAttribute('aria-expanded', String(aberto));
      toggle.querySelector('.rail__label').textContent = aberto ? 'Recolher' : 'Expandir';
    }
  }

  function marcarNavAtiva() {
    var caminho = window.location.pathname;
    var itens = document.querySelectorAll('.rail__item[data-nav]');

    for (var i = 0; i < itens.length; i++) {
      if (itens[i].getAttribute('data-nav') === caminho) {
        itens[i].setAttribute('aria-current', 'page');
      }
    }
  }

  // ------------------------------------------------------------------
  // Fleet meter
  // ------------------------------------------------------------------

  var FleetMeter = (function () {
    var MAX_SEGMENTOS = 60;

    function render(counts) {
      var barra = document.getElementById('meter-bar');
      var leitura = document.getElementById('meter-reading');
      if (!barra || !counts) { return; }

      var estados = []
        .concat(preencher('running', counts.running))
        .concat(preencher('stopped', counts.stopped))
        .concat(preencher('disabled', counts.disabled));

      // Frotas grandes: um segmento por emulador viraria um borrão. Acima
      // do teto, os segmentos passam a ser proporcionais.
      if (estados.length > MAX_SEGMENTOS) {
        estados = amostrar(estados, MAX_SEGMENTOS);
      }

      barra.textContent = '';
      estados.forEach(function (estado) {
        var seg = document.createElement('span');
        seg.className = 'meter__seg';
        seg.setAttribute('data-state', estado);
        barra.appendChild(seg);
      });

      barra.setAttribute('aria-label',
        counts.running + ' de ' + counts.total + ' emuladores ativos, ' +
        counts.stopped + ' parados, ' + counts.disabled + ' desabilitados');

      leitura.innerHTML = '';
      var ativos = document.createElement('b');
      ativos.textContent = String(counts.running);
      leitura.appendChild(ativos);
      leitura.appendChild(document.createTextNode(' / ' + counts.total));
    }

    function preencher(estado, quantidade) {
      var saida = [];
      for (var i = 0; i < quantidade; i++) { saida.push(estado); }
      return saida;
    }

    function amostrar(itens, tamanho) {
      var saida = [];
      var passo = itens.length / tamanho;
      for (var i = 0; i < tamanho; i++) {
        saida.push(itens[Math.floor(i * passo)]);
      }
      return saida;
    }

    return { render: render };
  })();

  function iniciarMeter() {
    var meter = document.getElementById('fleet-meter');
    var saude = document.getElementById('meter-health');
    if (!meter) { return; }

    // Semeia com o que o servidor renderizou, para a barra não aparecer
    // vazia antes do snapshot chegar.
    FleetMeter.render({
      total: Number(meter.getAttribute('data-total')),
      running: Number(meter.getAttribute('data-running')),
      stopped: Number(meter.getAttribute('data-stopped')),
      disabled: Number(meter.getAttribute('data-disabled'))
    });

    window.FleetStream.subscribe('snapshot', function (dados) {
      FleetMeter.render(dados.counts);
    });

    window.FleetStream.subscribe('device', function (dados) {
      FleetMeter.render(dados.counts);
    });

    window.FleetStream.subscribe('status', function (estado) {
      // Reconectando não apaga os números: só avisa que podem estar
      // defasados. O comportamento antigo zerava tudo e escrevia
      // "Offline" na primeira falha de rede.
      if (estado === 'live') {
        saude.textContent = '';
      } else if (estado === 'reconnecting') {
        saude.textContent = 'reconectando…';
      } else {
        saude.textContent = 'sem conexão';
      }
    });
  }

  // ------------------------------------------------------------------
  // Ações globais
  // ------------------------------------------------------------------

  function postar(url, corpo) {
    return fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(corpo)
    }).then(function (resposta) {
      if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
      return resposta;
    });
  }

  function iniciarAcoesGlobais() {
    var iniciarTodos = document.getElementById('start-all');
    var pararTodos = document.getElementById('stop-all');
    var sincronizar = document.getElementById('sync-db');

    if (iniciarTodos) {
      iniciarTodos.addEventListener('click', function () {
        iniciarTodos.disabled = true;
        postar('/start', { devices: ['all'], enable_log: {} })
          .then(function () { window.Toast.ok('Iniciando todos os emuladores'); })
          .catch(function () { window.Toast.err('Não foi possível iniciar os emuladores'); })
          .then(function () { iniciarTodos.disabled = false; });
      });
    }

    if (pararTodos) {
      pararTodos.addEventListener('click', function () {
        pararTodos.disabled = true;
        postar('/stop', { devices: ['all'] })
          .then(function () { window.Toast.ok('Parando todos os emuladores'); })
          .catch(function () { window.Toast.err('Não foi possível parar os emuladores'); })
          .then(function () { pararTodos.disabled = false; });
      });
    }

    if (sincronizar) {
      sincronizar.addEventListener('click', function () {
        if (!window.confirm(
          'Sincronizar com o W-Access atualiza os dispositivos disponíveis e ' +
          'suas configurações. Confirme que o serviço dos gerenciadores ' +
          'virtuais está parado antes de continuar.')) {
          return;
        }
        sincronizarBanco(sincronizar);
      });
    }
  }

  function sincronizarBanco(botao) {
    botao.disabled = true;
    window.Toast.info('Sincronizando com o W-Access…');

    fetch('/refresh')
      .then(function (resposta) {
        if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
        return aguardarConclusao();
      })
      .then(function () {
        window.Toast.ok('Sincronização concluída. Recarregando…');
        window.setTimeout(function () { window.location.reload(); }, 800);
      })
      .catch(function () {
        window.Toast.err('Falha ao sincronizar com o W-Access');
        botao.disabled = false;
      });
  }

  // O refresh roda em background no servidor; /api/refresh-status é o
  // único jeito de saber que terminou.
  function aguardarConclusao() {
    var INTERVALO = 1000;
    var LIMITE = 60;

    return new Promise(function (resolve, reject) {
      var tentativas = 0;

      var timer = window.setInterval(function () {
        tentativas++;

        if (tentativas > LIMITE) {
          window.clearInterval(timer);
          reject(new Error('timeout'));
          return;
        }

        fetch('/api/refresh-status')
          .then(function (r) { return r.json(); })
          .then(function (dados) {
            if (dados.completed) {
              window.clearInterval(timer);
              resolve();
            }
          })
          .catch(function () { /* segue tentando */ });
      }, INTERVALO);
    });
  }

  function iniciarLogo() {
    var logo = document.getElementById('brand-logo');
    var fallback = document.getElementById('brand-logo-fallback');
    if (!logo || !fallback) { return; }

    logo.addEventListener('error', function () {
      logo.hidden = true;
      fallback.hidden = false;
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    iniciarRail();
    marcarNavAtiva();
    iniciarLogo();
    iniciarMeter();
    iniciarAcoesGlobais();
    window.FleetStream.start();
  });

  window.FleetMeter = FleetMeter;
})();
```

- [ ] **Step 8: Verificar que o CDN sumiu do shell**

```bash
grep -rn "https://\|jquery\|bootstrap\|bi bi-\|fa fa-" assets/web/templates/base.html assets/web/templates/header.html assets/web/templates/sidebar.html assets/web/templates/footer.html
```

Expected: nenhuma saída.

- [ ] **Step 9: Subir a aplicação e verificar o shell**

```bash
go build ./... && echo "build ok"
node --check assets/web/static/js/app.js && echo "sintaxe ok"
```

Expected: `build ok` e `sintaxe ok`. As páginas de conteúdo ainda usam
classes Bootstrap e vão parecer quebradas até as Tasks 7–10; o shell
(topbar, rail, meter, footer) é o que se verifica aqui.

- [ ] **Step 10: Commit**

```bash
git add assets/web/static/css/layout.css assets/web/static/css/components.css assets/web/static/js/app.js assets/web/templates
git commit -m "feat(ui): shell do console com fleet meter, rail persistente e zero CDN"
```

---

### Task 7: Página de dispositivos

**Execute as Tasks 7 e 8 em sequência, sem parar no meio.** A reescrita de
`devices.html` remove o modal Bootstrap de detalhes, e `device-details.js`
depende de `bootstrap.Modal`, que já não existe depois da Task 6. Entre o
commit da Task 7 e o da Task 8 o botão Detalhes fica inoperante — a Task 8
é o que devolve a funcionalidade, portada para o drawer. O botão em si
continua na tabela desde já, com o mesmo seletor e o mesmo
`data-device-id`, para o gatilho não precisar mudar duas vezes.

**Files:**
- Modify: `assets/web/templates/devices.html` (reescrita completa)
- Create: `assets/web/static/js/devices.js`

**Interfaces:**
- Consumes: `window.FleetStream`, `window.Toast`, tokens e componentes das Tasks 4–6
- Consumes: contexto do `mainPage` — `.devices` (com `lc_id`, `name`, `model`, `port`, `log_enabled`, `status`, `interval`, `total`), `.page`, `.total_pages`, `.per_page`, `.page_range`, `.filters`
- Produces: `window.DeviceDrawer.open(id, nome)` — ponto de entrada consumido pela Task 8; nesta tarefa é um stub que a Task 8 preenche

- [ ] **Step 1: Reescrever `assets/web/templates/devices.html`**

```html
{{ template "base.html" . }}
{{ define "title" }}Dispositivos - Facial Emulators{{ end }}
{{ define "content" }}

<div class="page-head">
  <h1>Dispositivos</h1>
  <div class="page-head__actions">
    <button type="button" class="btn btn--go" id="start-selected" disabled>
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#play"></use></svg>
      Iniciar selecionados
    </button>
    <button type="button" class="btn btn--halt" id="stop-selected" disabled>
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#stop"></use></svg>
      Parar selecionados
    </button>
  </div>
</div>

<!-- Filtros sempre visíveis. O card colapsável antigo custava dois cliques
     e mais um estado para três campos de texto. -->
<form class="filters" id="filter-form" method="get" action="/">
  <div class="field">
    <label class="field__label" for="filter-id">LocalControllerID</label>
    <input class="input input--mono" type="number" id="filter-id" name="id"
           placeholder="123" value="{{ .filters.id }}">
  </div>
  <div class="field">
    <label class="field__label" for="filter-name">Nome</label>
    <input class="input" type="text" id="filter-name" name="name"
           placeholder="Portaria" value="{{ .filters.name }}">
  </div>
  <div class="field">
    <label class="field__label" for="filter-port">Porta</label>
    <input class="input input--mono" type="number" id="filter-port" name="port"
           placeholder="7070" value="{{ .filters.port }}">
  </div>
  <input type="hidden" name="per_page" value="{{ .per_page }}">
  <div class="filters__actions">
    <button type="submit" class="btn btn--action">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#search"></use></svg>
      Filtrar
    </button>
    <a class="btn" href="/?per_page={{ .per_page }}">Limpar</a>
  </div>
</form>

<div class="table-wrap">
  <table class="grid" id="device-grid">
    <thead>
      <tr>
        <th class="center">
          <input type="checkbox" class="check" id="select-all"
                 aria-label="Selecionar todos os dispositivos">
        </th>
        <th>ID</th>
        <th>Nome</th>
        <th>Modelo</th>
        <th class="num">Porta</th>
        <th class="center">Log</th>
        <th class="num">Intervalo</th>
        <th class="num">Usuários</th>
        <th>Estado</th>
        <th class="center">Ações</th>
      </tr>
    </thead>
    <tbody>
      {{ range .devices }}
      <tr id="device-{{ .lc_id }}" data-id="{{ .lc_id }}" data-state="{{ .status }}"
          data-name="{{ .name }}">
        <td class="center">
          <input type="checkbox" class="check device-check"
                 aria-label="Selecionar {{ .name }}">
        </td>
        <td>
          <span class="grid__id">
            <span class="led" data-state="{{ .status }}" aria-hidden="true"></span>
            {{ .lc_id }}
          </span>
        </td>
        <td class="grid__name device-name-cell" title="{{ .name }}">{{ .name }}</td>
        <td>{{ .model }}</td>
        <td class="num">{{ .port }}</td>
        <td class="center">
          <input type="checkbox" class="check log-check" data-id="{{ .lc_id }}"
                 aria-label="Habilitar log para {{ .name }}"
                 {{ if eq .log_enabled 1 }}checked{{ end }}
                 {{ if ne .status "stopped" }}disabled{{ end }}>
        </td>
        <td class="num">{{ .interval }}s</td>
        <td class="num">{{ .total }}</td>
        <td>
          <span class="badge" data-state="{{ .status }}">
            <span class="led" data-state="{{ .status }}" aria-hidden="true"></span>
            {{ if eq .status "running" }}ativo{{ else if eq .status "disabled" }}desabilitado{{ else }}parado{{ end }}
          </span>
        </td>
        <td>
          <!-- <button disabled>, não <a class="disabled">: o link falso
               continuava focável e acionável por teclado. -->
          <div class="row-actions">
            <button type="button" class="btn btn--sm btn--go device-start"
                    data-id="{{ .lc_id }}" title="Iniciar"
                    {{ if ne .status "stopped" }}disabled{{ end }}>
              <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#play"></use></svg>
              <span class="visually-hidden">Iniciar {{ .name }}</span>
            </button>
            <button type="button" class="btn btn--sm btn--halt device-stop"
                    data-id="{{ .lc_id }}" title="Parar"
                    {{ if ne .status "running" }}disabled{{ end }}>
              <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#stop"></use></svg>
              <span class="visually-hidden">Parar {{ .name }}</span>
            </button>
            <!-- Detalhes existe hoje e não pode sumir: abre usuários e
                 configurações do dispositivo (Task 8). -->
            <button type="button" class="btn btn--sm device-details-btn"
                    data-device-id="{{ .lc_id }}" title="Acessar dados do dispositivo">
              <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#users"></use></svg>
              <span class="visually-hidden">Detalhes de {{ .name }}</span>
            </button>
          </div>
        </td>
      </tr>
      {{ else }}
      <tr>
        <td colspan="10">
          <div class="empty">
            <p class="empty__title">Nenhum dispositivo encontrado</p>
            <p>Ajuste os filtros ou sincronize com o W-Access para trazer os dispositivos.</p>
            <a class="btn btn--action" href="/?per_page={{ .per_page }}">Limpar filtros</a>
          </div>
        </td>
      </tr>
      {{ end }}
    </tbody>
  </table>
</div>

<div class="pager">
  <div class="field">
    <label class="field__label" for="per-page">Itens por página</label>
    <select class="select" id="per-page">
      <option value="5" {{ if eq .per_page 5 }}selected{{ end }}>5</option>
      <option value="10" {{ if eq .per_page 10 }}selected{{ end }}>10</option>
      <option value="20" {{ if eq .per_page 20 }}selected{{ end }}>20</option>
      <option value="50" {{ if eq .per_page 50 }}selected{{ end }}>50</option>
    </select>
  </div>

  <nav class="pager__pages" aria-label="Paginação">
    <a class="pager__page" href="/?page={{ sub .page 1 }}&per_page={{ .per_page }}{{ if .filters.id }}&id={{ .filters.id }}{{ end }}{{ if .filters.name }}&name={{ .filters.name }}{{ end }}{{ if .filters.port }}&port={{ .filters.port }}{{ end }}"
       {{ if eq .page 1 }}aria-disabled="true"{{ end }} aria-label="Página anterior">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#chevron-left"></use></svg>
    </a>
    {{ range $p := .page_range }}
    <a class="pager__page" href="/?page={{ $p }}&per_page={{ $.per_page }}{{ if $.filters.id }}&id={{ $.filters.id }}{{ end }}{{ if $.filters.name }}&name={{ $.filters.name }}{{ end }}{{ if $.filters.port }}&port={{ $.filters.port }}{{ end }}"
       {{ if eq $.page $p }}aria-current="page"{{ end }}>{{ $p }}</a>
    {{ end }}
    <a class="pager__page" href="/?page={{ add .page 1 }}&per_page={{ .per_page }}{{ if .filters.id }}&id={{ .filters.id }}{{ end }}{{ if .filters.name }}&name={{ .filters.name }}{{ end }}{{ if .filters.port }}&port={{ .filters.port }}{{ end }}"
       {{ if eq .page .total_pages }}aria-disabled="true"{{ end }} aria-label="Próxima página">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#chevron-right"></use></svg>
    </a>
  </nav>
</div>

{{ end }}

{{ define "additional_js" }}
<script src="/static/js/devices.js"></script>
{{ end }}
```

- [ ] **Step 2: Escrever `assets/web/static/js/devices.js`**

```js
/**
 * devices.js — tabela de dispositivos.
 *
 * Todo o estado de uma linha (LED, badge, botões, checkbox de log) é
 * derivado de um único data-state, aplicado por pintarLinha(). Antes essa
 * lógica existia duplicada: uma vez no template Go e outra numa template
 * string de JS, que iam divergindo.
 */
(function () {
  'use strict';

  var ROTULOS = { running: 'ativo', stopped: 'parado', disabled: 'desabilitado', error: 'erro' };

  // ------------------------------------------------------------------
  // Render de linha
  // ------------------------------------------------------------------

  function pintarLinha(id, estado) {
    var linha = document.getElementById('device-' + id);
    if (!linha) { return; }

    var anterior = linha.getAttribute('data-state');
    linha.setAttribute('data-state', estado);

    var leds = linha.querySelectorAll('.led');
    for (var i = 0; i < leds.length; i++) {
      leds[i].setAttribute('data-state', estado);
      if (anterior !== estado) { piscar(leds[i]); }
    }

    var badge = linha.querySelector('.badge');
    if (badge) {
      badge.setAttribute('data-state', estado);
      // Só o nó de texto: recriar o innerHTML jogaria fora o LED interno.
      var texto = badge.lastChild;
      if (texto && texto.nodeType === Node.TEXT_NODE) {
        texto.nodeValue = ' ' + (ROTULOS[estado] || estado);
      }
    }

    var iniciar = linha.querySelector('.device-start');
    var parar = linha.querySelector('.device-stop');
    var log = linha.querySelector('.log-check');

    if (iniciar) {
      iniciar.disabled = estado !== 'stopped';
      iniciar.removeAttribute('data-pending');
    }
    if (parar) {
      parar.disabled = estado !== 'running';
      parar.removeAttribute('data-pending');
    }
    if (log) { log.disabled = estado !== 'stopped'; }
  }

  function piscar(led) {
    led.classList.remove('led--flash');
    // Força reflow para a animação reiniciar em mudanças consecutivas.
    void led.offsetWidth;
    led.classList.add('led--flash');
  }

  function atualizarContagem(id, totalUsuarios) {
    var linha = document.getElementById('device-' + id);
    if (!linha) { return; }
    var celulas = linha.querySelectorAll('td.num');
    // Ordem das colunas numéricas: porta, intervalo, usuários.
    if (celulas.length >= 3) {
      celulas[2].textContent = String(totalUsuarios);
    }
  }

  // ------------------------------------------------------------------
  // Seleção
  // ------------------------------------------------------------------

  function selecionados() {
    var ids = [];
    var checks = document.querySelectorAll('.device-check:checked');
    for (var i = 0; i < checks.length; i++) {
      ids.push(checks[i].closest('tr').getAttribute('data-id'));
    }
    return ids;
  }

  function logsSelecionados() {
    var mapa = {};
    var checks = document.querySelectorAll('.device-check:checked');
    for (var i = 0; i < checks.length; i++) {
      var linha = checks[i].closest('tr');
      var log = linha.querySelector('.log-check');
      mapa[linha.getAttribute('data-id')] = log ? log.checked : false;
    }
    return mapa;
  }

  function sincronizarSelecao() {
    var todos = document.querySelectorAll('.device-check');
    var marcados = document.querySelectorAll('.device-check:checked');
    var selectAll = document.getElementById('select-all');

    if (selectAll) {
      selectAll.checked = todos.length > 0 && marcados.length === todos.length;
      selectAll.indeterminate = marcados.length > 0 && marcados.length < todos.length;
    }

    // Botões em lote nascem desabilitados: um clique sem seleção antes
    // devolvia um alert("Nenhum dispositivo selecionado"), que é um erro
    // que a interface podia ter evitado.
    var vazio = marcados.length === 0;
    document.getElementById('start-selected').disabled = vazio;
    document.getElementById('stop-selected').disabled = vazio;
  }

  // ------------------------------------------------------------------
  // Ações
  // ------------------------------------------------------------------

  function postar(url, corpo) {
    return fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(corpo)
    }).then(function (resposta) {
      if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
      return resposta;
    });
  }

  function acaoLinha(botao, url, corpo, mensagem) {
    botao.setAttribute('data-pending', 'true');

    postar(url, corpo)
      .then(function () { window.Toast.ok(mensagem); })
      .catch(function () {
        botao.removeAttribute('data-pending');
        window.Toast.err('A operação falhou. Verifique o log do serviço.');
      });
    // O estado final chega pelo SSE, que é quem sabe o resultado de
    // verdade; pintarLinha() limpa o data-pending.
  }

  function iniciarAcoes() {
    document.addEventListener('click', function (evento) {
      var iniciar = evento.target.closest('.device-start');
      if (iniciar) {
        var idIniciar = iniciar.getAttribute('data-id');
        var log = document.querySelector('#device-' + idIniciar + ' .log-check');
        var mapaLog = {};
        mapaLog[idIniciar] = log ? log.checked : false;
        acaoLinha(iniciar, '/start', { devices: [idIniciar], enable_log: mapaLog },
          'Iniciando emulador ' + idIniciar);
        return;
      }

      var parar = evento.target.closest('.device-stop');
      if (parar) {
        var idParar = parar.getAttribute('data-id');
        acaoLinha(parar, '/stop', { devices: [idParar] },
          'Parando emulador ' + idParar);
        return;
      }

      // Mesmo seletor e mesmo data-attribute que a implementação atual
      // usa, para o botão Detalhes continuar funcionando durante a
      // transição — a Task 8 substitui o alvo, não o gatilho.
      var detalhes = evento.target.closest('.device-details-btn');
      if (detalhes && window.DeviceDrawer) {
        var linha = detalhes.closest('tr');
        var celulaNome = linha ? linha.querySelector('.device-name-cell') : null;
        window.DeviceDrawer.open(
          detalhes.getAttribute('data-device-id'),
          celulaNome ? celulaNome.textContent.trim() : ''
        );
      }
    });

    document.getElementById('start-selected').addEventListener('click', function () {
      var ids = selecionados();
      postar('/start', { devices: ids, enable_log: logsSelecionados() })
        .then(function () { window.Toast.ok('Iniciando ' + ids.length + ' emulador(es)'); })
        .catch(function () { window.Toast.err('Não foi possível iniciar os emuladores'); });
    });

    document.getElementById('stop-selected').addEventListener('click', function () {
      var ids = selecionados();
      postar('/stop', { devices: ids })
        .then(function () { window.Toast.ok('Parando ' + ids.length + ' emulador(es)'); })
        .catch(function () { window.Toast.err('Não foi possível parar os emuladores'); });
    });
  }

  function iniciarSelecao() {
    var selectAll = document.getElementById('select-all');
    if (selectAll) {
      selectAll.addEventListener('change', function () {
        var checks = document.querySelectorAll('.device-check');
        for (var i = 0; i < checks.length; i++) {
          checks[i].checked = selectAll.checked;
        }
        sincronizarSelecao();
      });
    }

    document.addEventListener('change', function (evento) {
      if (evento.target.classList.contains('device-check')) {
        sincronizarSelecao();
      }
    });

    sincronizarSelecao();
  }

  function iniciarPaginacao() {
    var perPage = document.getElementById('per-page');
    if (!perPage) { return; }

    perPage.addEventListener('change', function () {
      var params = new URLSearchParams(window.location.search);
      params.set('page', '1');
      params.set('per_page', perPage.value);
      window.location.search = params.toString();
    });
  }

  function iniciarStream() {
    window.FleetStream.subscribe('snapshot', function (dados) {
      dados.devices.forEach(function (device) {
        pintarLinha(device.id, device.status);
        atualizarContagem(device.id, device.total_users);
      });
    });

    window.FleetStream.subscribe('device', function (dados) {
      if (!dados.device) { return; }
      pintarLinha(dados.device.id, dados.device.status);
      atualizarContagem(dados.device.id, dados.device.total_users);
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    iniciarSelecao();
    iniciarAcoes();
    iniciarPaginacao();
    iniciarStream();
  });
})();
```

- [ ] **Step 3: Verificar que nada de Bootstrap sobrou**

```bash
node --check assets/web/static/js/devices.js && echo "sintaxe ok"
grep -n "https://\|class=\"btn btn-outline\|bi bi-\|col-md\|d-flex\|<style>\|<script>[^<]*function" assets/web/templates/devices.html
```

Expected: `sintaxe ok`, e o `grep` retornando apenas a linha do
`<script src="/static/js/devices.js">` do bloco `additional_js`.

- [ ] **Step 4: Verificar na aplicação rodando**

Subir o serviço conforme `DEV-GUIDE.md` e abrir `http://localhost:7070/`.
Conferir, com o painel de rede aberto:

1. Exatamente **uma** requisição a `/events`, em estado pendente.
2. Nenhuma requisição a host externo.
3. As contagens do meter batem com a tabela assim que a página carrega.
4. Iniciar um emulador em outra aba faz o LED piscar e o meter mudar nesta.
5. Um filtro sem resultado mostra o empty state.
6. `Tab` não alcança botão de dispositivo desabilitado.

- [ ] **Step 5: Commit**

```bash
git add assets/web/templates/devices.html assets/web/static/js/devices.js
git commit -m "feat(ui): tabela de dispositivos com LED por linha, ações em lote e empty state"
```

---

### Task 8: Drawer de detalhe do dispositivo (porte da UI existente)

**Esta tarefa é um porte, não uma construção do zero.** Já existe uma UI
de detalhes funcionando — um modal Bootstrap em `devices.html` mais
`assets/web/static/js/device-details.js` — e ela **não pode regredir**. A
tarefa move esse comportamento para o drawer do console, com paridade
funcional obrigatória e as melhorias listadas abaixo.

**Paridade obrigatória.** Tudo o que a implementação atual faz precisa
continuar funcionando:

| Comportamento atual | Onde está hoje |
|---|---|
| Título `<nome> — LC <id>` | `device-details.js`, `openDeviceDetails` |
| Abas Usuários / Configurações | modal, `nav-tabs` |
| Busca de usuário com debounce de 300ms | `device-details.js`, listener de `input` |
| Colunas de usuário: ID, Nome, Cartão, Face, Validade | modal, `<thead>` |
| Face como badge Sim/Não | `renderDeviceUsers` |
| Validade formatada `DD-MM-AAAA` em UTC, `—` quando vazia | `formatValidity` |
| Cartão vazio vira `—` | `renderDeviceUsers` |
| Resumo `<primeiro>-<último> de <total>` | `renderUsersPagination` |
| Anterior/Próxima desabilitados nos extremos | `renderUsersPagination` |
| Configurações: colunas Configuração / Valor | modal, aba settings |
| Estados carregando / vazio / erro, por aba, com a mensagem do erro | ambas as funções `load*` |
| Mensagem de vazio distingue "sem usuários" de "busca sem resultado" | `renderDeviceUsers` |

**Melhorias permitidas nesta tarefa** (e só estas):

1. Modal Bootstrap → drawer do console. O Bootstrap sai na Task 6, então o
   `bootstrap.Modal` deixa de existir de qualquer forma.
2. `innerHTML` + `escapeHtml()` → construção de nós com `textContent`. O
   helper `escapeHtml` existia porque os templates rodavam em
   `text/template`; com a Task 2 a origem foi corrigida, e no JS o certo é
   nunca montar HTML por concatenação.
3. Fechar com `Esc`, foco devolvido ao botão de origem, LED do drawer
   acompanhando o estado em tempo real.
4. Um toggle de **log habilitado**, gravado via `PUT /api/devices/:id/settings`.

**Fora de escopo, e a razão:** editar o intervalo de eventos. O handler
`updateDeviceSettings` aceita **apenas** `{"log_enabled": bool}` e
`Manager.UpdateDeviceSettings(id int, logEnabled bool)` só grava essa
coluna. Tornar o intervalo editável exige mudar manager, handler e
migration — trabalho de backend que não pertence a um redesign de UI.
A aba Configurações continua sendo leitura do que o gerenciador gravou,
como é hoje.

**Files:**
- Create: `assets/web/static/js/device-drawer.js`
- Delete: `assets/web/static/js/device-details.js` (substituído; nunca foi commitado)
- Modify: `assets/web/templates/devices.html` (markup do drawer + tag de script)

**Interfaces:**
- Consumes: `GET /api/devices/:id/users?page=&per_page=&search=` → `{ users, total, page, per_page, model }`, com `users[]` no formato `database.DeviceUserRow`: `{ id: string, name: string, card_no: string, has_face: bool, valid_to: string }`
- Consumes: `GET /api/devices/:id/settings` → `{ settings }`, com `settings[]` no formato `database.DeviceSettingRow`: `{ cfg_id: string, value: string }`
- Consumes: `PUT /api/devices/:id/settings` com corpo `{"log_enabled": bool}`
- Produces: `window.DeviceDrawer.open(id, nome)`, `window.DeviceDrawer.close()`

- [ ] **Step 1: Reler a implementação que está sendo portada**

```bash
cd "c:/Personal Development/Personal-Projects/GoFacialEmulator"
cat assets/web/static/js/device-details.js
grep -n "DeviceUserRow" -A 8 internal/database/device_inspect.go
```

Os nomes de campo já estão fixados no bloco **Interfaces** acima, lidos do
código real. Este passo existe para conferir a tabela de paridade item a
item antes de escrever o substituto — qualquer comportamento da lista que
não aparecer no novo arquivo é uma regressão.

- [ ] **Step 2: Adicionar o markup do drawer em `devices.html`**

Inserir imediatamente antes do `{{ end }}` que fecha o bloco `content`:

```html
<div class="drawer-scrim" id="drawer-scrim" hidden></div>

<aside class="drawer" id="device-drawer" data-open="false"
       role="dialog" aria-modal="true" aria-labelledby="drawer-title" hidden>
  <div class="drawer__head">
    <h2 class="drawer__title" id="drawer-title">
      <span class="led" id="drawer-led" aria-hidden="true"></span>
      <span id="drawer-name">Dispositivo</span>
    </h2>
    <button type="button" class="btn btn--icon drawer__close" id="drawer-close"
            aria-label="Fechar">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#close"></use></svg>
    </button>
  </div>

  <div class="drawer__tabs" role="tablist">
    <button type="button" class="drawer__tab" role="tab" id="tab-users"
            data-tab="users" aria-selected="true" aria-controls="panel-users">Usuários</button>
    <button type="button" class="drawer__tab" role="tab" id="tab-settings"
            data-tab="settings" aria-selected="false" aria-controls="panel-settings">Configurações</button>
  </div>

  <div class="drawer__body">
    <!-- Aba Usuários -->
    <section id="panel-users" role="tabpanel" aria-labelledby="tab-users">
      <input class="input" type="search" id="users-search"
             placeholder="Buscar por nome ou ID" aria-label="Buscar usuário">

      <div class="table-wrap" style="margin-top: var(--gap-3)">
        <table class="grid">
          <thead>
            <tr>
              <th>ID</th>
              <th>Nome</th>
              <th>Cartão</th>
              <th class="center">Face</th>
              <th class="num">Validade</th>
            </tr>
          </thead>
          <tbody id="users-body"></tbody>
        </table>
      </div>

      <div class="pager">
        <span class="meter__reading" id="users-summary"></span>
        <div class="pager__pages">
          <button type="button" class="btn btn--sm" id="users-prev">Anterior</button>
          <button type="button" class="btn btn--sm" id="users-next">Próxima</button>
        </div>
      </div>
    </section>

    <!-- Aba Configurações -->
    <section id="panel-settings" role="tabpanel" aria-labelledby="tab-settings" hidden>
      <div class="panel" style="max-width: none">
        <div class="panel__head">Emulador</div>
        <div class="panel__body">
          <label class="field" style="flex-direction: row; align-items: center; gap: var(--gap-2)">
            <input type="checkbox" class="check" id="drawer-log">
            <span>Gravar log de eventos</span>
          </label>
          <button type="button" class="btn btn--action" id="drawer-save-log"
                  style="align-self: flex-start">Salvar</button>
        </div>
      </div>

      <p class="label">Configurações gravadas pelo gerenciador</p>
      <div class="table-wrap">
        <table class="grid">
          <thead>
            <tr>
              <th>Configuração</th>
              <th>Valor</th>
            </tr>
          </thead>
          <tbody id="settings-body"></tbody>
        </table>
      </div>
    </section>
  </div>
</aside>
```

O markup das duas abas é estático, como no modal atual: só as linhas das
tabelas e o resumo são construídos em JS.

E na tag de script do bloco `additional_js`, acrescentar antes de
`devices.js` — o drawer precisa estar registrado quando `devices.js` liga
o handler de clique:

```html
<script src="/static/js/device-drawer.js"></script>
<script src="/static/js/devices.js"></script>
```

- [ ] **Step 3: Escrever `assets/web/static/js/device-drawer.js`**

Porte de `device-details.js`. Cada comportamento da tabela de paridade
aparece aqui; a diferença de fundo é que nada é montado por concatenação de
HTML — o helper `escapeHtml()` do arquivo antigo desaparece porque não há
mais string de HTML para escapar.

```js
/**
 * device-drawer.js — detalhe de um dispositivo: usuários gravados no
 * emulador e configurações do gerenciador.
 *
 * Porte do modal Bootstrap de device-details.js para o drawer do console.
 * O comportamento é o mesmo; o que muda é que as linhas são construídas
 * como nós com textContent em vez de template strings de HTML — por isso
 * o antigo escapeHtml() não tem equivalente aqui.
 */
(function () {
  'use strict';

  var POR_PAGINA = 10;
  var DEBOUNCE_BUSCA = 300;

  var estado = {
    id: null,
    nome: '',
    pagina: 1,
    total: 0,
    busca: '',
    timerBusca: null
  };

  var focoAnterior = null;

  function el(id) { return document.getElementById(id); }

  // ------------------------------------------------------------------
  // Abrir / fechar
  // ------------------------------------------------------------------

  function abrir(id, nome) {
    estado.id = id;
    estado.nome = nome || '';
    estado.pagina = 1;
    estado.busca = '';
    estado.total = 0;

    el('users-search').value = '';
    el('drawer-name').textContent = estado.nome + ' — LC ' + id;

    var linha = el('device-' + id);
    el('drawer-led').setAttribute('data-state',
      linha ? linha.getAttribute('data-state') : 'stopped');

    focoAnterior = document.activeElement;

    var drawer = el('device-drawer');
    drawer.hidden = false;
    el('drawer-scrim').hidden = false;
    // Próximo frame: o elemento precisa estar visível para a transição de
    // transform acontecer.
    window.requestAnimationFrame(function () {
      drawer.setAttribute('data-open', 'true');
    });

    selecionarAba('users');

    // As duas abas carregam na abertura, como no modal antigo: trocar de
    // aba fica instantâneo.
    carregarUsuarios();
    carregarConfiguracoes();

    el('drawer-close').focus();
  }

  function fechar() {
    var drawer = el('device-drawer');
    drawer.setAttribute('data-open', 'false');
    el('drawer-scrim').hidden = true;

    window.setTimeout(function () { drawer.hidden = true; }, 200);

    if (focoAnterior && focoAnterior.focus) { focoAnterior.focus(); }
    estado.id = null;
  }

  function selecionarAba(aba) {
    var abas = document.querySelectorAll('.drawer__tab');
    for (var i = 0; i < abas.length; i++) {
      abas[i].setAttribute('aria-selected',
        String(abas[i].getAttribute('data-tab') === aba));
    }

    el('panel-users').hidden = aba !== 'users';
    el('panel-settings').hidden = aba !== 'settings';
  }

  // ------------------------------------------------------------------
  // Mensagens de estado dentro de uma tabela
  // ------------------------------------------------------------------

  function mensagemNaTabela(tbody, colunas, texto, tipo) {
    tbody.textContent = '';

    var tr = document.createElement('tr');
    var td = document.createElement('td');
    td.colSpan = colunas;

    var caixa = document.createElement('div');
    caixa.className = 'empty';
    if (tipo === 'erro') { caixa.style.color = 'var(--halt)'; }
    caixa.textContent = texto;

    td.appendChild(caixa);
    tr.appendChild(td);
    tbody.appendChild(tr);
  }

  // ------------------------------------------------------------------
  // Usuários
  // ------------------------------------------------------------------

  function carregarUsuarios() {
    var tbody = el('users-body');
    mensagemNaTabela(tbody, 5, 'Carregando…');

    var params = new URLSearchParams({
      page: String(estado.pagina),
      per_page: String(POR_PAGINA),
      search: estado.busca
    });

    fetch('/api/devices/' + estado.id + '/users?' + params.toString())
      .then(function (resposta) {
        if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
        return resposta.json();
      })
      .then(function (dados) {
        estado.total = dados.total || 0;
        renderUsuarios(dados.users || []);
        renderPaginacao();
      })
      .catch(function (erro) {
        mensagemNaTabela(tbody, 5,
          'Erro ao carregar usuários: ' + erro.message, 'erro');
        el('users-summary').textContent = '';
        el('users-prev').disabled = true;
        el('users-next').disabled = true;
      });
  }

  function renderUsuarios(usuarios) {
    var tbody = el('users-body');

    if (usuarios.length === 0) {
      mensagemNaTabela(tbody, 5, estado.busca
        ? 'Nenhum usuário encontrado para a busca'
        : 'Nenhum usuário gravado neste dispositivo');
      return;
    }

    tbody.textContent = '';

    usuarios.forEach(function (usuario) {
      var tr = document.createElement('tr');

      tr.appendChild(celula(usuario.id, 'mono'));
      tr.appendChild(celula(usuario.name, ''));
      tr.appendChild(celula(usuario.card_no, ''));
      tr.appendChild(celulaFace(usuario.has_face));
      tr.appendChild(celula(formatarValidade(usuario.valid_to), 'num'));

      tbody.appendChild(tr);
    });
  }

  function celula(valor, classe) {
    var td = document.createElement('td');
    if (classe) { td.className = classe; }

    var texto = valor === null || valor === undefined ? '' : String(valor);
    if (texto === '') {
      td.textContent = '—';
      td.style.color = 'var(--text-low)';
    } else {
      td.textContent = texto;
    }

    return td;
  }

  function celulaFace(temFace) {
    var td = document.createElement('td');
    td.className = 'center';

    var badge = document.createElement('span');
    badge.className = 'badge';
    badge.setAttribute('data-state', temFace ? 'running' : 'disabled');
    badge.textContent = temFace ? 'Sim' : 'Não';

    td.appendChild(badge);
    return td;
  }

  // Mesma formatação do modal antigo: DD-MM-AAAA em UTC. Ler em UTC evita
  // a data mudar de dia conforme o fuso de quem abre a tela.
  function formatarValidade(valor) {
    if (!valor) { return ''; }

    var data = new Date(valor);
    if (isNaN(data.getTime())) { return String(valor); }

    var dia = String(data.getUTCDate()).padStart(2, '0');
    var mes = String(data.getUTCMonth() + 1).padStart(2, '0');
    return dia + '-' + mes + '-' + data.getUTCFullYear();
  }

  function renderPaginacao() {
    var primeiro = estado.total === 0 ? 0 : (estado.pagina - 1) * POR_PAGINA + 1;
    var ultimo = Math.min(estado.pagina * POR_PAGINA, estado.total);

    el('users-summary').textContent = estado.total === 0
      ? ''
      : primeiro + '-' + ultimo + ' de ' + estado.total;

    el('users-prev').disabled = estado.pagina <= 1;
    el('users-next').disabled = ultimo >= estado.total;
  }

  // ------------------------------------------------------------------
  // Configurações
  // ------------------------------------------------------------------

  function carregarConfiguracoes() {
    var tbody = el('settings-body');
    mensagemNaTabela(tbody, 2, 'Carregando…');

    var linha = el('device-' + estado.id);
    var log = linha ? linha.querySelector('.log-check') : null;
    el('drawer-log').checked = log ? log.checked : false;
    // O emulador precisa estar parado para trocar a flag de log — mesma
    // regra que a tabela aplica na coluna Log.
    var parado = linha && linha.getAttribute('data-state') === 'stopped';
    el('drawer-log').disabled = !parado;
    el('drawer-save-log').disabled = !parado;

    fetch('/api/devices/' + estado.id + '/settings')
      .then(function (resposta) {
        if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
        return resposta.json();
      })
      .then(function (dados) {
        renderConfiguracoes(dados.settings || []);
      })
      .catch(function (erro) {
        mensagemNaTabela(tbody, 2,
          'Erro ao carregar configurações: ' + erro.message, 'erro');
      });
  }

  function renderConfiguracoes(settings) {
    var tbody = el('settings-body');

    if (settings.length === 0) {
      mensagemNaTabela(tbody, 2, 'Nenhuma configuração gravada');
      return;
    }

    tbody.textContent = '';

    settings.forEach(function (item) {
      var tr = document.createElement('tr');
      tr.appendChild(celula(item.cfg_id, ''));
      tr.appendChild(celula(item.value, 'mono'));
      tbody.appendChild(tr);
    });
  }

  function salvarLog() {
    var botao = el('drawer-save-log');
    botao.disabled = true;

    fetch('/api/devices/' + estado.id + '/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ log_enabled: el('drawer-log').checked })
    })
      .then(function (resposta) {
        if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
        window.Toast.ok('Configuração de log salva');

        // Mantém a coluna Log da tabela em sincronia, sem recarregar.
        var linha = el('device-' + estado.id);
        var log = linha ? linha.querySelector('.log-check') : null;
        if (log) { log.checked = el('drawer-log').checked; }
      })
      .catch(function () {
        window.Toast.err('Não foi possível salvar a configuração de log');
      })
      .then(function () { botao.disabled = false; });
  }

  // ------------------------------------------------------------------
  // Ligação
  // ------------------------------------------------------------------

  document.addEventListener('DOMContentLoaded', function () {
    el('drawer-close').addEventListener('click', fechar);
    el('drawer-scrim').addEventListener('click', fechar);
    el('drawer-save-log').addEventListener('click', salvarLog);

    var abas = document.querySelectorAll('.drawer__tab');
    for (var i = 0; i < abas.length; i++) {
      abas[i].addEventListener('click', function (evento) {
        selecionarAba(evento.currentTarget.getAttribute('data-tab'));
      });
    }

    // Debounce de 300ms: digitar não deve disparar uma consulta por tecla.
    el('users-search').addEventListener('input', function (evento) {
      var valor = evento.target.value;
      window.clearTimeout(estado.timerBusca);
      estado.timerBusca = window.setTimeout(function () {
        estado.busca = valor.trim();
        estado.pagina = 1;
        carregarUsuarios();
      }, DEBOUNCE_BUSCA);
    });

    el('users-prev').addEventListener('click', function () {
      if (estado.pagina > 1) {
        estado.pagina -= 1;
        carregarUsuarios();
      }
    });

    el('users-next').addEventListener('click', function () {
      if (estado.pagina * POR_PAGINA < estado.total) {
        estado.pagina += 1;
        carregarUsuarios();
      }
    });

    document.addEventListener('keydown', function (evento) {
      if (evento.key === 'Escape' && estado.id !== null) { fechar(); }
    });

    // O LED do drawer e a disponibilidade do toggle de log acompanham o
    // dispositivo aberto em tempo real.
    window.FleetStream.subscribe('device', function (dados) {
      if (!dados.device || String(dados.device.id) !== String(estado.id)) { return; }

      el('drawer-led').setAttribute('data-state', dados.device.status);

      var parado = dados.device.status === 'stopped';
      el('drawer-log').disabled = !parado;
      el('drawer-save-log').disabled = !parado;
    });
  });

  window.DeviceDrawer = { open: abrir, close: fechar };
})();
```

- [ ] **Step 4: Remover o arquivo substituído**

```bash
rm assets/web/static/js/device-details.js
grep -rn "device-details" assets/ internal/
```

Expected: nenhuma saída. O arquivo nunca foi commitado, então não há
`git rm` — some do working tree e pronto. Qualquer referência remanescente
em `devices.html` é resto da Task 7 e precisa sair.

- [ ] **Step 5: Verificar a paridade item a item**

```bash
node --check assets/web/static/js/device-drawer.js && echo "sintaxe ok"
```

Com a aplicação rodando, percorrer a tabela de paridade do topo desta
tarefa. Cada linha é um teste:

1. Abrir um dispositivo: título mostra `<nome> — LC <id>`.
2. A lista de usuários traz ID, Nome, Cartão, Face e Validade; Face é badge
   Sim/Não; validade aparece como `DD-MM-AAAA`; cartão vazio vira `—`.
3. Digitar na busca dispara **uma** requisição depois de parar de digitar,
   não uma por tecla (conferir no painel de rede).
4. O resumo mostra `1-10 de N`; Anterior fica desabilitado na página 1 e
   Próxima na última.
5. Busca sem resultado mostra "Nenhum usuário encontrado para a busca";
   dispositivo sem usuários mostra "Nenhum usuário gravado neste
   dispositivo" — são mensagens diferentes.
6. Aba Configurações lista Configuração/Valor; sem registros mostra
   "Nenhuma configuração gravada".
7. Derrubar o serviço e reabrir o drawer: cada aba mostra a mensagem de
   erro com o motivo, não uma tabela vazia.
8. Em um dispositivo parado, marcar o log e salvar: toast de sucesso e a
   coluna Log da tabela acompanha. Em um dispositivo rodando, o toggle está
   desabilitado.
9. `Esc` fecha; o foco volta ao botão Detalhes que abriu.

- [ ] **Step 6: Commit**

```bash
git add assets/web/static/js/device-drawer.js assets/web/templates/devices.html
git commit -m "feat(ui): porta o detalhe de dispositivo para o drawer do console"
```

---

### Task 9: Página de comparação

**Files:**
- Modify: `assets/web/templates/comparison.html` (reescrita completa)
- Create: `assets/web/static/js/comparison.js`

**Interfaces:**
- Consumes: contexto de `comparisonPage` — `.values` (com `site_controller_id`, `local_controller_id`, `name`, `port`, `wxs_total`, `site_controller_total`, `emulator_total`), `.page`, `.per_page`, `.total_pages`, `.page_range`
- Consumes: `GET /comparison_refresh`

- [ ] **Step 1: Reescrever `assets/web/templates/comparison.html`**

```html
{{ template "base.html" . }}
{{ define "title" }}Comparação - Facial Emulators{{ end }}
{{ define "content" }}

<div class="page-head">
  <h1>Comparação</h1>
  <div class="page-head__actions">
    <button type="button" class="btn btn--action" id="refresh-comparison">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#refresh"></use></svg>
      Recontar
    </button>
  </div>
</div>

<p class="panel__note">
  Compara o total de usuários no W-Access, no gerenciador e no emulador.
  Linhas destacadas têm divergência entre as três contagens.
</p>

<div class="table-wrap">
  <table class="grid" id="comparison-grid">
    <thead>
      <tr>
        <th class="num">SiteControllerID</th>
        <th class="num">LocalControllerID</th>
        <th>Nome</th>
        <th class="num">Porta</th>
        <th class="num">W-Access</th>
        <th class="num">Gerenciador</th>
        <th class="num">Emulador</th>
        <th>Estado</th>
      </tr>
    </thead>
    <tbody>
      {{ range .values }}
      {{ $divergente := or (ne .wxs_total .site_controller_total) (ne .wxs_total .emulator_total) (ne .site_controller_total .emulator_total) }}
      <tr data-state="{{ if $divergente }}error{{ else }}running{{ end }}">
        <td class="num">{{ .site_controller_id }}</td>
        <td class="num">{{ .local_controller_id }}</td>
        <td class="grid__name" title="{{ .name }}">{{ .name }}</td>
        <td class="num">{{ .port }}</td>
        <td class="num">{{ .wxs_total }}</td>
        <td class="num">{{ .site_controller_total }}</td>
        <td class="num">{{ .emulator_total }}</td>
        <td>
          {{ if $divergente }}
          <span class="badge" data-state="error">divergente</span>
          {{ else }}
          <span class="badge" data-state="running">conferido</span>
          {{ end }}
        </td>
      </tr>
      {{ else }}
      <tr>
        <td colspan="8">
          <div class="empty">
            <p class="empty__title">Nenhuma comparação disponível</p>
            <p>Use Recontar para comparar as contagens de usuários.</p>
          </div>
        </td>
      </tr>
      {{ end }}
    </tbody>
  </table>
</div>

<div class="pager">
  <div class="field">
    <label class="field__label" for="per-page">Itens por página</label>
    <select class="select" id="per-page">
      <option value="5" {{ if eq .per_page 5 }}selected{{ end }}>5</option>
      <option value="10" {{ if eq .per_page 10 }}selected{{ end }}>10</option>
      <option value="20" {{ if eq .per_page 20 }}selected{{ end }}>20</option>
      <option value="50" {{ if eq .per_page 50 }}selected{{ end }}>50</option>
    </select>
  </div>

  <nav class="pager__pages" aria-label="Paginação">
    <a class="pager__page" href="/comparison?page={{ sub .page 1 }}&per_page={{ .per_page }}"
       {{ if eq .page 1 }}aria-disabled="true"{{ end }} aria-label="Página anterior">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#chevron-left"></use></svg>
    </a>
    {{ range $p := .page_range }}
    <a class="pager__page" href="/comparison?page={{ $p }}&per_page={{ $.per_page }}"
       {{ if eq $.page $p }}aria-current="page"{{ end }}>{{ $p }}</a>
    {{ end }}
    <a class="pager__page" href="/comparison?page={{ add .page 1 }}&per_page={{ .per_page }}"
       {{ if eq .page .total_pages }}aria-disabled="true"{{ end }} aria-label="Próxima página">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#chevron-right"></use></svg>
    </a>
  </nav>
</div>

{{ end }}

{{ define "additional_js" }}
<script src="/static/js/comparison.js"></script>
{{ end }}
```

- [ ] **Step 2: Escrever `assets/web/static/js/comparison.js`**

```js
/**
 * comparison.js
 *
 * A versão anterior tinha duas definições de refresh_counter — uma que
 * fazia o fetch e outra que só dava alert("Refresh acionado!") — e nenhuma
 * delas informava o resultado nem recarregava a tabela. O botão parecia
 * não fazer nada.
 */
(function () {
  'use strict';

  function iniciarRefresh() {
    var botao = document.getElementById('refresh-comparison');
    if (!botao) { return; }

    botao.addEventListener('click', function () {
      if (!window.confirm(
        'A recontagem exige que o serviço dos gerenciadores virtuais esteja ' +
        'parado. Confirme antes de continuar.')) {
        return;
      }

      botao.disabled = true;
      window.Toast.info('Recontando usuários…');

      fetch('/comparison_refresh')
        .then(function (resposta) {
          if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
          window.Toast.ok('Recontagem concluída. Recarregando…');
          window.setTimeout(function () { window.location.reload(); }, 800);
        })
        .catch(function () {
          window.Toast.err('Não foi possível recontar os usuários');
          botao.disabled = false;
        });
    });
  }

  function iniciarPaginacao() {
    var perPage = document.getElementById('per-page');
    if (!perPage) { return; }

    perPage.addEventListener('change', function () {
      var params = new URLSearchParams(window.location.search);
      params.set('page', '1');
      params.set('per_page', perPage.value);
      window.location.search = params.toString();
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    iniciarRefresh();
    iniciarPaginacao();
  });
})();
```

- [ ] **Step 3: Adicionar a faixa de divergência ao CSS**

Acrescentar ao final de `assets/web/static/css/components.css`:

```css
/* Divergência na comparação: faixa na borda esquerda, não fundo inteiro.
   Fundo vermelho em linha larga compete com o texto e cansa a leitura. */
.grid tr[data-state='error'] td:first-child {
  box-shadow: inset 2px 0 0 var(--warn);
}

.grid tr[data-state='error'] td { background: color-mix(in srgb, var(--warn) 6%, transparent); }
```

- [ ] **Step 4: Verificar**

```bash
node --check assets/web/static/js/comparison.js && echo "sintaxe ok"
grep -n "https://\|bi bi-\|col-md\|<style>" assets/web/templates/comparison.html
```

Expected: `sintaxe ok` e nenhuma saída do `grep`.

- [ ] **Step 5: Commit**

```bash
git add assets/web/templates/comparison.html assets/web/static/js/comparison.js assets/web/static/css/components.css
git commit -m "feat(ui): página de comparação no console, com recontagem que informa o resultado"
```

---

### Task 10: Página de configurações

**Files:**
- Modify: `assets/web/templates/settings.html` (reescrita completa)
- Create: `assets/web/static/js/settings.js`

**Interfaces:**
- Consumes: `.wxs_settings` (campos `Host`, `Port`, `Database`, `Username`, `Password`)
- Consumes: `POST /api/settings/test-wxs-connection`, `POST /api/settings/wxs`

- [ ] **Step 1: Reescrever `assets/web/templates/settings.html`**

O campo de senha deixa de trazer a senha no atributo `value`: em vez
disso ele é vazio e a `placeholder` conta que há uma senha gravada. Quem
não digitar nada mantém a senha atual.

```html
{{ template "base.html" . }}
{{ define "title" }}Configurações - Facial Emulators{{ end }}
{{ define "content" }}

<div class="page-head">
  <h1>Configurações</h1>
</div>

<form class="panel" id="wxs-form">
  <div class="panel__head">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#devices"></use></svg>
    Conexão com o W-Access (SQL Server)
  </div>

  <div class="panel__body">
    <div class="field">
      <label class="field__label" for="wxs-host">Host ou endereço IP</label>
      <input class="input" type="text" id="wxs-host" name="host"
             value="{{ .wxs_settings.Host }}" required>
    </div>

    <div class="field">
      <label class="field__label" for="wxs-port">Porta</label>
      <input class="input input--mono" type="number" id="wxs-port" name="port"
             value="{{ .wxs_settings.Port }}" required>
    </div>

    <div class="field">
      <label class="field__label" for="wxs-database">Database</label>
      <input class="input" type="text" id="wxs-database" name="database"
             value="{{ .wxs_settings.Database }}" required>
    </div>

    <div class="field">
      <label class="field__label" for="wxs-username">Usuário</label>
      <input class="input" type="text" id="wxs-username" name="username"
             value="{{ .wxs_settings.Username }}" required>
    </div>

    <div class="field">
      <label class="field__label" for="wxs-password">Senha</label>
      <div class="input-group">
        <input class="input" type="password" id="wxs-password" name="password"
               placeholder="{{ if .wxs_settings.Password }}Senha gravada — deixe em branco para manter{{ else }}Informe a senha{{ end }}"
               autocomplete="off">
        <button type="button" class="btn btn--icon" id="toggle-password"
                aria-label="Mostrar senha" aria-pressed="false">
          <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#search"></use></svg>
        </button>
      </div>
    </div>

    <div class="filters__actions">
      <button type="submit" class="btn btn--action">Salvar</button>
      <button type="button" class="btn" id="test-connection">Testar conexão</button>
      <a class="btn" href="/">Cancelar</a>
    </div>

    <p class="panel__note" id="wxs-result" hidden></p>
  </div>
</form>

<div class="panel">
  <div class="panel__head">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#info"></use></svg>
    Banco do serviço (PostgreSQL)
  </div>
  <div class="panel__body">
    <p class="panel__note">
      As conexões <code>service_db</code> e <code>emulator_db</code> vêm de
      <code>configs/config.yaml</code> e não são editáveis por aqui.
    </p>
  </div>
</div>

{{ end }}

{{ define "additional_js" }}
<script src="/static/js/settings.js"></script>
{{ end }}
```

- [ ] **Step 2: Escrever `assets/web/static/js/settings.js`**

```js
/**
 * settings.js — configurações de conexão com o W-Access.
 *
 * As mensagens de resultado usam textContent e não innerHTML: o campo
 * result.error vem do servidor e pode conter a mensagem crua do driver.
 */
(function () {
  'use strict';

  function coletar() {
    return {
      host: document.getElementById('wxs-host').value,
      port: parseInt(document.getElementById('wxs-port').value, 10),
      database: document.getElementById('wxs-database').value,
      username: document.getElementById('wxs-username').value,
      password: document.getElementById('wxs-password').value
    };
  }

  function mostrarResultado(ok, mensagem) {
    var alvo = document.getElementById('wxs-result');
    alvo.hidden = false;
    alvo.textContent = mensagem;
    alvo.style.color = ok ? 'var(--live)' : 'var(--halt)';
  }

  function enviar(url, botao, rotuloOcupado, aoSucesso) {
    var rotuloOriginal = botao.textContent;
    botao.disabled = true;
    botao.textContent = rotuloOcupado;

    return fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(coletar())
    })
      .then(function (r) { return r.json(); })
      .then(function (resultado) {
        if (resultado.success) {
          aoSucesso();
        } else {
          mostrarResultado(false, resultado.error || 'A operação falhou.');
          window.Toast.err('A operação falhou');
        }
      })
      .catch(function (erro) {
        mostrarResultado(false, String(erro.message || erro));
        window.Toast.err('Não foi possível falar com o serviço');
      })
      .then(function () {
        botao.disabled = false;
        botao.textContent = rotuloOriginal;
      });
  }

  document.addEventListener('DOMContentLoaded', function () {
    var alternar = document.getElementById('toggle-password');
    var senha = document.getElementById('wxs-password');

    alternar.addEventListener('click', function () {
      var visivel = senha.type === 'text';
      senha.type = visivel ? 'password' : 'text';
      alternar.setAttribute('aria-pressed', String(!visivel));
      alternar.setAttribute('aria-label', visivel ? 'Mostrar senha' : 'Ocultar senha');
    });

    document.getElementById('test-connection').addEventListener('click', function () {
      enviar('/api/settings/test-wxs-connection', this, 'Testando…', function () {
        mostrarResultado(true, 'Conexão bem-sucedida.');
        window.Toast.ok('Conexão bem-sucedida');
      });
    });

    document.getElementById('wxs-form').addEventListener('submit', function (evento) {
      evento.preventDefault();
      var botao = this.querySelector('button[type="submit"]');

      enviar('/api/settings/wxs', botao, 'Salvando…', function () {
        mostrarResultado(true, 'Configurações salvas e conexão reiniciada.');
        window.Toast.ok('Configurações salvas');
      });
    });
  });
})();
```

- [ ] **Step 3: Confirmar o tratamento de senha em branco no backend**

```bash
awk '/func \(h \*Handler\) saveWxsSettings/,/^}/' internal/handlers/handlers.go
```

Se `saveWxsSettings` gravar a senha recebida sem checagem, uma senha em
branco apagaria a gravada — o que contradiz a `placeholder` do formulário.
Nesse caso, adicionar no início do handler, logo após o bind do JSON:

```go
	// Senha em branco significa "manter a atual": o formulário nunca
	// devolve a senha gravada ao browser, então um campo vazio é ausência
	// de mudança, não intenção de apagar.
	if settings.Password == "" {
		atual, err := database.LoadWxsSettings(context.Background(), h.serviceDB)
		if err == nil && atual != nil {
			settings.Password = atual.Password
		}
	}
```

Ajustar o nome da função de leitura para o que `settingsPage` já usa —
localizável com `grep -n "WxsSettings" internal/handlers/handlers.go`.

- [ ] **Step 4: Verificar**

```bash
node --check assets/web/static/js/settings.js && echo "sintaxe ok"
go build ./... && go test ./... 2>&1 | tail -10
grep -n "https://\|bi bi-\|col-lg\|<style>" assets/web/templates/settings.html
```

Expected: `sintaxe ok`, build e testes passando, `grep` sem saída.

- [ ] **Step 5: Commit**

```bash
git add assets/web/templates/settings.html assets/web/static/js/settings.js internal/handlers/handlers.go
git commit -m "feat(ui): configurações no console; senha em branco mantém a gravada"
```

---

### Task 11: Remover o front antigo e verificar os critérios de aceite

**Files:**
- Delete: `assets/web/static/css/header.css`, `assets/web/static/css/main.css`
- Delete: `assets/web/static/js/header.js`, `assets/web/static/js/main.js`
- Modify: `README.md`

- [ ] **Step 1: Confirmar que nada referencia os arquivos antigos**

```bash
cd "c:/Personal Development/Personal-Projects/GoFacialEmulator"
grep -rn "header.css\|main.css\|header.js\|main.js" assets/ internal/ cmd/ --include="*.html" --include="*.go" --include="*.js"
```

Expected: nenhuma saída. Qualquer resultado é uma referência pendente que
precisa ser resolvida antes de apagar.

- [ ] **Step 2: Apagar**

```bash
git rm assets/web/static/css/header.css assets/web/static/css/main.css \
       assets/web/static/js/header.js assets/web/static/js/main.js
```

`header.js` era a segunda conexão SSE e o polling de 15s; `main.js`
carregava a camada `window.App` que nunca foi usada por nenhuma página.

- [ ] **Step 3: Rodar os critérios de aceite verificáveis por comando**

```bash
echo "--- 1. sem host externo ---"
grep -rn "https://\|http://cdn\|jquery\|bootstrap" assets/web/templates/ assets/web/static/css/ assets/web/static/js/ || echo "OK: nenhum recurso externo"

echo "--- 2. sem style/script inline nos templates ---"
grep -rn "<style>\|onclick=\|onchange=\|onerror=" assets/web/templates/ || echo "OK: nenhum inline"

echo "--- 3. build e testes ---"
go build ./... && go test ./... 2>&1 | tail -15
```

Expected: as duas primeiras seções imprimem a linha `OK:`; a terceira,
build limpo e testes passando. A única exceção aceitável no item 2 são as
tags `<script src="...">` dos blocos `additional_js`, que o padrão de busca
acima não captura.

- [ ] **Step 4: Verificação manual no navegador**

Subir o serviço e percorrer os critérios que exigem olho:

1. `/` mostra as contagens corretas sem interação — o snapshot chega no connect.
2. Painel de rede: **uma** requisição a `/events`, pendente. Nenhuma a host externo.
3. Iniciar um emulador em outra aba: LED pisca e o meter muda em menos de 1s.
4. Derrubar o serviço: o meter mostra "reconectando…" e **mantém** os números. Subir de volta: volta ao normal sozinho, sem recarregar.
5. Filtrar por um nome inexistente: empty state com o botão Limpar filtros.
6. Parar o PostgreSQL e recarregar `/`: página de erro dentro do shell, não 500 branco.
7. `Tab` pela tabela: botões desabilitados não recebem foco.
8. Números de página aparecem entre as setas.
9. Reduzir a janela a 400px: a tabela rola horizontalmente, o shell não.
10. Com "reduzir movimento" ativo no SO: o LED troca de cor sem o flash.

- [ ] **Step 5: Atualizar o README**

Na seção que descreve a interface web, substituir qualquer menção a
Bootstrap ou jQuery por uma descrição do que passou a existir:

```markdown
### Interface web

Console escuro servido pelo próprio binário — todos os assets são
embutidos via `go:embed`, sem CDN e sem build step.

- **Tempo real:** uma conexão SSE por aba (`/events`), com snapshot no
  connect, keepalive de 20s e reconexão com backoff. O *fleet meter* na
  barra superior e o LED de cada linha da tabela refletem o estado da frota
  em tempo real.
- **Assets:** CSS próprio com tokens em `assets/web/static/css/tokens.css`,
  IBM Plex Sans/Mono como `woff2` locais e um sprite SVG de ícones.
- **Detalhe do dispositivo:** clicar no nome abre um painel com os usuários
  gravados e as configurações graváveis do emulador.
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore(ui): remove o front Bootstrap/jQuery antigo e atualiza o README"
```

---

## Self-Review

**Cobertura da spec.** Cada item numerado da spec tem tarefa:
R1 (Task 5, conexão única), R2/R3 (Tasks 6–7, meter e LED substituem os
IDs mortos), R4 (Task 3), R5 (Task 3), R6 (Task 5, `agendarFallback` nunca
zera as contagens), R7 (Task 5, fallback só após 30s), R8/R9 (Task 9),
R10 (Task 1). S1/S2/S3 (Tasks 5–7), S4 (Task 7, `data-pending`), S5
(Task 7, `{{ else }}`), S6 (Task 2), S7 (Task 7, `<button disabled>`), S8
(Task 6, `localStorage`), S9 (Task 1). V1–V3 (Task 4, tokens), V4 (Task 4,
tema único), V5 (Task 6, `data-rail`), V6 (Task 6, `.content` sem card
aninhado), V7 (Task 6, `.grid`), V8 (Task 6, o meter tem `aria-label`),
V9 (Task 6, `.table-wrap`), V10 (Task 6, `h1` em toda página), V11
(Task 10), V12 (Task 6), V13 (Task 7, coluna Modelo). D1/D2/D3 (Tasks 6 e
11), D4 (Task 2), D5 (Task 2), D6 (Tasks 6–10), D7 (Task 10), D8
(Tasks 5, 8, 10 — `textContent` em todo lugar).

**Dependências entre tarefas.** As Tasks 1–3 são backend e independentes
do front; 4–6 constroem a fundação visual; 7–10 dependem de 4–6; 11 depende
de todas. A Task 7 referencia `window.DeviceDrawer` sob guarda
(`if (abrir && window.DeviceDrawer)`), então funciona antes da Task 8
existir.

**Ponto que exige verificação durante a execução.** A Task 8, Step 1
manda ler `device_inspect.go` antes de escrever o render: os nomes de campo
de `ListDeviceUsers` e `ListDeviceSettings` não foram confirmados no
planejamento, e o código do Step 3 assume `name`/`card`/`employee_id` e
`key`/`value`. Se divergirem, o render se ajusta aos nomes reais. Do mesmo
modo, a Task 10, Step 3 verifica se `saveWxsSettings` precisa da guarda de
senha em branco antes de adicioná-la.
