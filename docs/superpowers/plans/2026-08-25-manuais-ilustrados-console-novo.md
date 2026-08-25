# Manuais Ilustrados sobre o Console Redesenhado — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Entregar três manuais em PDF — um por pacote de instalação — ilustrados com a interface redesenhada do console, escritos para técnico de campo, cobrindo desde o download do Docker Desktop e a instalação do WSL2 até o roteiro de validação com o W-Access, e gerados por um pipeline que qualquer estação consegue rodar.

**Architecture:** Antes de qualquer captura, uma tarefa de base reúne numa única branch a interface nova e o trabalho de alcançabilidade de rede e modo de dispositivo, reescrevendo a coluna Modo e o aviso de alcançabilidade no sistema de design do console. Depois disso: capítulos em Markdown em `docs/manual/conteudo/`, com os comuns escritos uma vez e referenciados pelos três alvos através de `manifesto.json`. Um script Node monta o HTML de cada alvo com capa, sumário e CSS de impressão; outro captura as telas do emulador e do W-Access com Playwright dirigindo o Chrome já instalado; um terceiro imprime o PDF pelo mesmo Playwright, com paged.js paginando dentro da página. Figuras que só um humano pode tirar viram caixa tracejada no PDF e linha em `CAPTURAS-PENDENTES.md`, sem quebrar o build.

**Tech Stack:** Go 1.21 (toolchain local 1.25.4), Gin, pgx/v4, `html/template` com cache no startup, Node 24 / npm 11, Playwright com `channel: 'chrome'` (usa o Chrome instalado, sem baixar browsers), paged.js, marked. Batch (`cmd.exe`) na integração com `packaging/build-pacotes.bat`.

**Spec:** `docs/superpowers/specs/2026-08-14-manuais-ilustrados-e-rede-design.md`, seções 8 e 11. **Onde a seção 11 divergir das seções 8.1, 8.4 e 8.5, ela prevalece** — ela existe justamente porque a interface mudou depois que a spec foi escrita.

**Substitui:** `docs/superpowers/plans/2026-08-14-manuais-ilustrados.md`, cuja base de execução (`feat/rede-e-modo-dispositivo`) ficou errada e cuja lista de seletores aponta para uma interface que não existe mais. Aquele plano nunca teve nenhuma tarefa executada; nada se perde ao substituí-lo.

## Global Constraints

- **Base de execução:** este plano roda sobre `feat/console-ui-redesign` **com** `feat/rede-e-modo-dispositivo` mesclada dentro dela, o que a Task 0 faz. Não executar a Task 0 invalida todas as capturas.
- Três dependências npm, todas em `docs/manual/package.json`, nenhuma no repositório Go: `playwright`, `pagedjs`, `marked`. Nenhuma outra é permitida.
- Playwright sempre com `channel: 'chrome'`. Nunca rodar `playwright install` — o Chrome da máquina é o browser.
- Todo texto que o usuário final lê é em português, sem jargão. O público é técnico de campo, não desenvolvedor. `SSE`, `template`, `handler` e `endpoint` não aparecem no texto do manual.
- Nenhuma credencial em arquivo versionado. O login do W-Access vem de `docs/manual/.captura.env`, que entra no `.gitignore`, com `.captura.env.exemplo` versionado ao lado.
- Capítulo declarado no manifesto e ausente do disco **falha** o build. Figura ausente **não** falha: vira placeholder e entra no checklist.
- Nenhum número de capítulo, seção ou figura é escrito à mão no Markdown. A numeração é gerada.
- Screenshots capturados vão versionados para `docs/manual/ativos/img/gerado/`. O build gera os PDFs a partir do que está versionado, sem exigir ambiente vivo.
- Captura é passo manual e deliberado (`npm run capturar`). O alvo `manuais` do build **não** captura.
- Comentários de código em português. Commits com título em inglês no formato Conventional Commits e corpo em português.
- **Rotas de página que existem e podem ser fotografadas: apenas `/` (dispositivos), `/settings` e `/comparison`.** Não existe rota de página para métricas — não tentar capturá-la.
- Nenhum seletor de callout é inventado: todos os usados neste plano foram lidos de `assets/web/templates/`. Seletor que não casa **derruba** a captura, por desenho.
- O console não usa CDN nem framework de CSS de terceiros. Todo estilo novo sai dos tokens em `assets/web/static/css/tokens.css` e dos componentes em `components.css`. Classes Bootstrap (`alert`, `btn-link`, `text-muted`, `bi bi-*`) **não existem** nesta interface e não podem ser reintroduzidas.

## Ambiente necessário

As Tasks 3 e 4 capturam telas de ambiente vivo. Diferente do plano anterior, aqui **o ambiente sobe antes**, e as figuras do emulador saem reais na primeira entrega. Pré-condições, verificadas no Step 1 da Task 3:

- Docker Desktop em execução, PostgreSQL de pé, aplicação respondendo em `http://localhost:7070`.
- Frota com dispositivos suficientes para a grade, os filtros e a paginação renderizarem com conteúdo representativo. Grade vazia não ilustra nada.
- **Pelo menos um dispositivo de modelo Dahua na frota.** A coluna Modo só exibe o seletor para dispositivos Dahua; para os demais mostra um traço. Sem um Dahua, a figura da coluna Modo vira uma coluna de traços e não ilustra o que o capítulo descreve.

O W-Access é opcional e independente: sem `docs/manual/.captura.env` ou sem servidor alcançável, a suíte dele é pulada com aviso, as figuras correspondentes viram pendência registrada, e a suíte do emulador roda até o fim.

---

### Task 0: Base de execução — coluna Modo e aviso de alcançabilidade no console novo

Nenhuma branch contém, ao mesmo tempo, a interface redesenhada e o trabalho das fases B e C. Capturar de qualquer uma das duas isoladamente produz um manual que descreve um produto que ninguém tem. Esta tarefa reúne as duas.

O backend da branch de rede entra sem conflito — `internal/reachability/`, `internal/handlers/device_mode.go`, `internal/handlers/reachability.go` são arquivos novos. O conflito é de apresentação: a coluna Modo e o aviso de alcançabilidade foram escritos em Bootstrap sobre `assets/web/static/js/main.js`, um arquivo que a interface nova apagou. **Reescrever nos componentes do console, não colar o HTML antigo.**

**Files:**
- Modify: `assets/web/templates/devices.html`
- Modify: `assets/web/static/js/devices.js`
- Modify: `assets/web/static/css/components.css`
- Test: `internal/handlers/render_test.go`

**Interfaces:**
- Consumes: `GET /api/reachability` (de `internal/handlers/reachability.go`, responde `reachability.Report`), o campo `local_auth` de cada dispositivo na listagem, e `POST /api/devices/:id/mode` — todos vindos de `feat/rede-e-modo-dispositivo`.
- Produces: a coluna Modo com `select.device-mode[data-device-id]` dentro de `#device-grid`, e o bloco de aviso `#reachability-alert` dentro de `#reachability-alert-row`, ambos no design do console. As Tasks 3 e 5 fotografam e descrevem exatamente esses identificadores.

- [ ] **Step 1: Criar a worktree a partir da branch da interface**

O trabalho não acontece na cópia de trabalho principal. Use a skill `superpowers:using-git-worktrees` se estiver disponível; o equivalente manual:

```bash
git worktree add ../GoFacialEmulator-manuais feat/console-ui-redesign
cd ../GoFacialEmulator-manuais
git switch -c feat/manuais-console
```

Todo o restante deste plano roda dentro dessa worktree.

- [ ] **Step 2: Mesclar a branch de rede e modo**

```bash
git merge feat/rede-e-modo-dispositivo
```

Espere conflito em `assets/web/templates/devices.html` e a ressurreição de `assets/web/static/js/main.js` (a interface nova o dividiu em `app.js`, `devices.js`, `device-drawer.js`, `realtime.js`, `settings.js`, `comparison.js`, `toast.js`).

Resolução, por arquivo:

- `assets/web/templates/devices.html` — **fique com a versão da interface nova**: `git checkout --ours -- assets/web/templates/devices.html`. O conteúdo da outra ponta é reescrito nos Steps 6 a 8, não mesclado.
- `assets/web/static/js/main.js` — **não recrie o arquivo**. Se o merge o trouxer de volta, apague: `git rm -f assets/web/static/js/main.js`. O comportamento dele vai para `devices.js` no Step 8.
- Qualquer arquivo `.go`, `packaging/*` ou `docs/*` — aceite a mescla normal. São arquivos novos ou sem sobreposição.

```bash
git add -A
git commit --no-edit
```

- [ ] **Step 3: Confirmar que o backend das duas pontas está vivo**

Run: `go build ./... && go test ./internal/reachability/... ./internal/handlers/... -v`
Expected: PASS. Em particular `TestRenderPage_Erro`, `TestBuildTemplateCache_CobreTodasAsPáginas` e `TestRenderPage_EscapaHTML` da interface nova, e os testes de `device_mode_test.go` e `reachability_http_test.go` da branch de rede.

Se algum teste de `handlers` falhar por template ausente, o Step 2 resolveu o conflito errado — refaça ficando com o `devices.html` da interface nova.

- [ ] **Step 4: Escrever os testes que falham**

Acrescentar ao fim de `internal/handlers/render_test.go`. Os dois testes renderizam a página de dispositivos com contexto representativo e cobram a marcação de que as Tasks 3 e 5 dependem:

```go
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
		"per_page":      50,
		"counter_cards": gin.H{"total": 2, "running": 1, "stopped": 1, "disabled": 0},
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
		"per_page":      50,
		"counter_cards": gin.H{"total": 0, "running": 0, "stopped": 0, "disabled": 0},
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
```

- [ ] **Step 5: Rodar os testes e confirmar que falham**

Run: `go test ./internal/handlers/ -run 'TestRenderDevices' -v`
Expected: FAIL nos dois, com `a grade não tem cabeçalho da coluna Modo` e `marcador ausente na página de dispositivos: id="reachability-alert-row"`.

- [ ] **Step 6: Escrever a coluna Modo no template**

Em `assets/web/templates/devices.html`, acrescentar o cabeçalho `<th>Modo</th>` logo depois de `<th class="num">Porta</th>`, e a célula correspondente logo depois da célula `<td class="num">{{ .port }}</td>`:

```html
        <td class="center">
          <!-- LocalAuthentication só é lido pelo emulador Dahua; os demais
               modelos ignoram a configuração. Um select ali seria um
               controle que não faz nada. -->
          {{ if eq .model "Dahua" }}
          <select class="select device-mode" data-device-id="{{ .lc_id }}"
                  aria-label="Modo de {{ .name }}">
            <option value="online" {{ if eq .local_auth "online" }}selected{{ end }}>Online</option>
            <option value="standalone" {{ if eq .local_auth "standalone" }}selected{{ end }}>Standalone</option>
          </select>
          {{ else }}
          <span class="muted" title="Este modelo não usa esta configuração">&mdash;</span>
          {{ end }}
        </td>
```

- [ ] **Step 7: Escrever o bloco de aviso e estilizá-lo com os tokens do console**

Ainda em `devices.html`, logo acima de `<form class="filters" id="filter-form" ...>`:

```html
<!-- A falha mais cara do emulador não emite sinal nenhum: os emuladores
     sobem, a tela fica toda verde e o W-Access mostra tudo offline. O aviso
     só existe quando há o que avisar — devices.js o revela. -->
<div id="reachability-alert-row" hidden>
  <div class="notice notice--warn" id="reachability-alert" role="alert">
    <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#alert"></use></svg>
    <div class="notice__body">
      <p class="notice__headline" id="reachability-headline"></p>
      <p class="notice__reason" id="reachability-reason"></p>
      <p class="notice__help" id="reachability-help"></p>
      <button type="button" class="btn btn--sm" id="reachability-toggle">ver a lista</button>
      <ul class="notice__list" id="reachability-list" hidden></ul>
    </div>
  </div>
</div>
```

Se `icons.svg` não tiver um símbolo `alert`, acrescente um com o mesmo estilo dos existentes. Confira antes: `grep -o 'id="[a-z-]*"' assets/web/static/icons.svg`.

Em `assets/web/static/css/components.css`, o componente `notice`, usando apenas variáveis já definidas em `tokens.css`. Confira os nomes disponíveis antes de escrever: `grep -o '\-\-[a-z0-9-]*' assets/web/static/css/tokens.css | sort -u`.

```css
/* Aviso de alcançabilidade. Um bloco só, sem variantes especulativas:
   hoje existe um único aviso na aplicação. */
.notice {
  display: flex;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  margin-bottom: var(--space-4);
  border: 1px solid var(--warn-border);
  border-radius: var(--radius-md);
  background: var(--warn-bg);
  color: var(--warn-fg);
}

.notice__headline { font-weight: 600; }
.notice__reason,
.notice__help { color: var(--text-dim); font-size: var(--fs-sm); }
.notice__list { margin-top: var(--space-2); font-size: var(--fs-sm); }
```

Se algum token acima não existir com esse nome em `tokens.css`, use o equivalente que existe — **não** invente um token novo nem escreva a cor literal.

- [ ] **Step 8: Ligar o comportamento em `devices.js`**

Duas responsabilidades, ambas em `assets/web/static/js/devices.js`, escritas no estilo do arquivo (delegação de evento a partir do documento, `fetch` com erro reportado por toast):

```js
// Troca o modo de um dispositivo. O select só existe para modelos Dahua.
document.addEventListener('change', async (e) => {
  const alvo = e.target.closest('.device-mode')
  if (!alvo) return

  const id = alvo.dataset.deviceId
  const anterior = alvo.dataset.anterior ?? alvo.value

  try {
    const r = await fetch(`/api/devices/${id}/mode`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode: alvo.value })
    })
    if (!r.ok) throw new Error(await r.text())
    alvo.dataset.anterior = alvo.value
    toast(`Dispositivo ${id}: modo ${alvo.value}`)
  } catch {
    // Volta o select ao valor anterior: deixar o valor novo na tela sem ter
    // gravado faria a tela mentir sobre o estado do dispositivo.
    alvo.value = anterior
    toast(`Não foi possível trocar o modo do dispositivo ${id}`, 'erro')
  }
})

// Preenche o aviso de alcançabilidade. Sem dispositivo problemático o bloco
// continua escondido.
async function carregarAlcancabilidade() {
  const linha = document.getElementById('reachability-alert-row')
  if (!linha) return

  let r
  try {
    r = await fetch('/api/reachability')
    if (!r.ok) return
  } catch {
    return
  }

  const rel = await r.json()
  const problematicos = (rel.devices ?? []).filter((d) => d.status !== 'reachable')
  if (problematicos.length === 0) return

  document.getElementById('reachability-headline').textContent =
    `${problematicos.length} dispositivo(s) não vão ser alcançados pelo Site Controller`
  document.getElementById('reachability-reason').textContent = rel.reason ?? ''
  document.getElementById('reachability-help').textContent =
    'Veja o capítulo Portas e rede do manual.'

  const lista = document.getElementById('reachability-list')
  lista.replaceChildren(
    ...problematicos.map((d) => {
      const li = document.createElement('li')
      li.textContent = `Dispositivo ${d.device_id} — porta ${d.port}`
      return li
    })
  )

  document.getElementById('reachability-toggle').addEventListener('click', () => {
    lista.hidden = !lista.hidden
  })

  linha.hidden = false
}

carregarAlcancabilidade()
```

Confira a forma real da resposta antes de escrever o filtro: `git show feat/rede-e-modo-dispositivo:internal/reachability/reachability.go`. Os nomes de campo do JSON (`devices`, `status`, `reason`, `device_id`, `port`) e o nome do estado saudável (`reachable`) têm de bater com as tags do struct `Report`; ajuste o código acima se divergirem. O mesmo vale para a rota de troca de modo — confirme o caminho real em `internal/handlers/handlers.go` da branch de rede.

- [ ] **Step 9: Rodar os testes e confirmar que passam**

Run: `go test ./internal/handlers/ -run 'TestRender' -v`
Expected: PASS nos cinco.

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 10: Conferir na tela**

Suba a aplicação e abra `http://localhost:7070`. A coluna Modo aparece entre Porta e Log, com seletor nos dispositivos Dahua e traço nos demais. O aviso de alcançabilidade aparece **apenas** se houver dispositivo inalcançável — se o ambiente estiver saudável, não aparece, e isso está certo.

- [ ] **Step 11: Commit**

```bash
git add assets/web/templates/devices.html assets/web/static/js/devices.js assets/web/static/css/components.css internal/handlers/render_test.go
git commit -m "$(cat <<'EOF'
feat(ui): mode column and reachability notice on the new console

As duas peças nasceram em feat/rede-e-modo-dispositivo escritas em
Bootstrap sobre um main.js que o redesenho apagou. Reescritas nos
componentes do console e nos tokens do tema, não coladas.

Os testes de render cobram a marcação que os manuais fotografam, e
reprovam classe de Bootstrap reintroduzida.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 1: Esqueleto do pipeline e montagem do HTML

O coração do pipeline. Depois desta tarefa é possível montar um alvo inteiro a partir de capítulos Markdown, com sumário e numeração, e o comportamento de placeholder já funciona.

**Files:**
- Create: `docs/manual/package.json`
- Create: `docs/manual/manifesto.json`
- Create: `docs/manual/ferramentas/montar.mjs`
- Create: `docs/manual/ativos/css/manual.css`
- Create: `docs/manual/conteudo/comum/.gitkeep`
- Create: `docs/manual/ativos/img/gerado/.gitkeep`, `docs/manual/ativos/img/manual/.gitkeep`, `docs/manual/ativos/img/svg/.gitkeep`
- Test: `docs/manual/ferramentas/montar.test.mjs`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: nada.
- Produces:
  - `montar(alvo, opcoes) -> { html, pendencias }` exportado por `montar.mjs`, onde `pendencias` é um array de `{ capitulo, alt, caminho }`
  - `docs/manual/.out/<alvo>.html` e `docs/manual/CAPTURAS-PENDENTES.md` quando executado pela linha de comando
  - Formato de `manifesto.json`, consumido pelas Tasks 5 a 8

- [ ] **Step 1: Criar o `package.json` e instalar**

`docs/manual/package.json`:

```json
{
  "name": "gofacialemulator-manuais",
  "private": true,
  "type": "module",
  "scripts": {
    "montar": "node ferramentas/montar.mjs",
    "capturar": "node ferramentas/capturar.mjs",
    "pdf": "node ferramentas/pdf.mjs",
    "manuais": "node ferramentas/montar.mjs && node ferramentas/pdf.mjs",
    "test": "node --test ferramentas/"
  },
  "dependencies": {
    "marked": "^12.0.0",
    "pagedjs": "^0.4.3",
    "playwright": "^1.44.0"
  }
}
```

Run: `cd docs/manual && npm install --ignore-scripts`
Expected: instala as três dependências. `--ignore-scripts` evita que o Playwright baixe browsers — usamos o Chrome da máquina.

Acrescentar ao `.gitignore` da raiz do repositório:

```gitignore
# Manuais: dependências, saída de montagem e credenciais de captura
docs/manual/node_modules/
docs/manual/.out/
docs/manual/.captura.env
```

- [ ] **Step 2: Escrever o manifesto**

`docs/manual/manifesto.json`. Os arquivos de capítulo ainda não existem — as Tasks 5 a 8 os criam. Declarar apenas os três alvos com seus títulos, e uma lista de capítulos vazia por enquanto:

```json
{
  "alvos": {
    "docker": {
      "titulo": "GoFacialEmulator — Manual do pacote Docker",
      "subtitulo": "Instalacao, configuracao e validacao",
      "capitulos": []
    },
    "windows": {
      "titulo": "GoFacialEmulator — Manual do pacote Windows",
      "subtitulo": "Instalacao, configuracao e validacao",
      "capitulos": []
    },
    "linux": {
      "titulo": "GoFacialEmulator — Manual do pacote Linux e WSL2",
      "subtitulo": "Instalacao, configuracao e validacao",
      "capitulos": []
    }
  }
}
```

Cada entrada de `capitulos` será uma string com o caminho relativo a `conteudo/`, por exemplo `"docker/instalar-docker-desktop.md"`.

- [ ] **Step 3: Escrever o teste que falha**

`docs/manual/ferramentas/montar.test.mjs`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, mkdir, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { montar } from './montar.mjs'

// cenario cria uma arvore de manual descartavel, para os testes nao
// dependerem do conteudo real que ainda esta sendo escrito.
async function cenario(capitulos, arquivos, imagens = []) {
  const raiz = await mkdtemp(join(tmpdir(), 'manual-'))
  await mkdir(join(raiz, 'conteudo', 'comum'), { recursive: true })
  await mkdir(join(raiz, 'ativos', 'css'), { recursive: true })
  await mkdir(join(raiz, 'ativos', 'img', 'manual'), { recursive: true })

  await writeFile(join(raiz, 'ativos', 'css', 'manual.css'), 'body{}')
  await writeFile(
    join(raiz, 'manifesto.json'),
    JSON.stringify({ alvos: { teste: { titulo: 'T', subtitulo: 'S', capitulos } } })
  )

  for (const [caminho, conteudo] of Object.entries(arquivos)) {
    await writeFile(join(raiz, 'conteudo', caminho), conteudo)
  }
  for (const nome of imagens) {
    await writeFile(join(raiz, 'ativos', 'img', 'manual', nome), 'png-de-mentira')
  }
  return raiz
}

test('capitulos entram na ordem do manifesto', async () => {
  const raiz = await cenario(
    ['comum/b.md', 'comum/a.md'],
    { 'comum/a.md': '# Segundo\n\ntexto a\n', 'comum/b.md': '# Primeiro\n\ntexto b\n' }
  )

  const { html } = await montar('teste', { raiz })

  assert.ok(html.indexOf('Primeiro') < html.indexOf('Segundo'),
    'a ordem do manifesto deve mandar, nao a ordem alfabetica')
})

test('capitulos sao numerados automaticamente', async () => {
  const raiz = await cenario(
    ['comum/a.md', 'comum/b.md'],
    { 'comum/a.md': '# Antes de comecar\n', 'comum/b.md': '# Instalar\n' }
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /1\.\s*Antes de comecar/)
  assert.match(html, /2\.\s*Instalar/)
})

test('sumario lista todos os capitulos', async () => {
  const raiz = await cenario(
    ['comum/a.md', 'comum/b.md'],
    { 'comum/a.md': '# Antes de comecar\n', 'comum/b.md': '# Instalar\n' }
  )

  const { html } = await montar('teste', { raiz })
  const sumario = html.slice(html.indexOf('id="sumario"'), html.indexOf('id="conteudo"'))

  assert.match(sumario, /Antes de comecar/)
  assert.match(sumario, /Instalar/)
})

test('capitulo declarado e ausente do disco falha o build', async () => {
  const raiz = await cenario(['comum/nao-existe.md'], {})

  await assert.rejects(
    () => montar('teste', { raiz }),
    /nao-existe\.md/,
    'a mensagem precisa dizer qual capitulo faltou'
  )
})

test('figura existente vira img, figura ausente vira placeholder e pendencia', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    {
      'comum/a.md':
        '# Instalar\n\n' +
        '![Tela que existe](img/manual/existe.png)\n\n' +
        '![Tela do wizard do Docker Desktop](img/manual/falta.png)\n'
    },
    ['existe.png']
  )

  const { html, pendencias } = await montar('teste', { raiz })

  assert.match(html, /<img[^>]+existe\.png/)
  assert.doesNotMatch(html, /<img[^>]+falta\.png/)
  assert.match(html, /figura-pendente/)
  assert.match(html, /Tela do wizard do Docker Desktop/)

  assert.equal(pendencias.length, 1)
  assert.equal(pendencias[0].alt, 'Tela do wizard do Docker Desktop')
  assert.match(pendencias[0].caminho, /falta\.png$/)
})

test('figuras sao numeradas e o placeholder tambem conta', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    {
      'comum/a.md':
        '# Instalar\n\n![Primeira](img/manual/existe.png)\n\n![Segunda](img/manual/falta.png)\n'
    },
    ['existe.png']
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /Figura 1\.1/)
  assert.match(html, /Figura 1\.2/)
})

test('alvo inexistente falha com mensagem clara', async () => {
  const raiz = await cenario([], {})
  await assert.rejects(() => montar('nao-existe', { raiz }), /nao-existe/)
})
```

- [ ] **Step 4: Rodar o teste e confirmar que falha**

Run: `cd docs/manual && npm test`
Expected: FAIL — `Cannot find module './montar.mjs'`.

- [ ] **Step 5: Escrever o `montar.mjs`**

`docs/manual/ferramentas/montar.mjs`:

```js
// Monta o HTML de um alvo a partir dos capitulos declarados no manifesto.
//
// Duas regras que valem a pena saber antes de mexer:
//   - capitulo declarado e ausente do disco FALHA o build. Sumico de
//     capitulo e erro, nao divida.
//   - figura ausente NAO falha: vira caixa tracejada e entra na lista de
//     pendencias. Se falhasse, o primeiro PDF nunca seria gerado, porque
//     algumas telas so um humano consegue tirar.
import { readFile, writeFile, mkdir, access } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { marked } from 'marked'

const AQUI = dirname(fileURLToPath(import.meta.url))
const RAIZ_PADRAO = resolve(AQUI, '..')

async function existe(caminho) {
  try {
    await access(caminho)
    return true
  } catch {
    return false
  }
}

function escaparHtml(s) {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

// renderizarCapitulo converte um capitulo em HTML e resolve as figuras.
// Devolve tambem as pendencias encontradas, para o checklist.
async function renderizarCapitulo(raiz, caminhoRelativo, numeroCapitulo) {
  const caminhoAbsoluto = join(raiz, 'conteudo', caminhoRelativo)
  if (!(await existe(caminhoAbsoluto))) {
    throw new Error(
      `capitulo declarado no manifesto mas ausente do disco: ${caminhoRelativo}`
    )
  }

  const markdown = await readFile(caminhoAbsoluto, 'utf8')
  const pendencias = []
  let numeroFigura = 0
  let titulo = caminhoRelativo

  // marked chama renderer.image de forma sincrona, entao a existencia de
  // cada figura precisa ser resolvida ANTES de montar o renderer.
  const figurasExistentes = new Set()
  for (const href of [...markdown.matchAll(/!\[[^\]]*\]\(([^)]+)\)/g)].map((m) => m[1])) {
    const caminhoImagem = join(raiz, 'ativos', href.replace(/^ativos\//, ''))
    if (await existe(caminhoImagem)) figurasExistentes.add(caminhoImagem)
  }

  const renderer = new marked.Renderer()

  renderer.heading = (texto, nivel) => {
    if (nivel === 1) {
      titulo = texto
      return `<h1 id="cap-${numeroCapitulo}">${numeroCapitulo}. ${texto}</h1>\n`
    }
    return `<h${nivel}>${texto}</h${nivel}>\n`
  }

  renderer.image = (href, _tituloImg, alt) => {
    numeroFigura += 1
    const rotulo = `Figura ${numeroCapitulo}.${numeroFigura}`
    const caminhoImagem = join(raiz, 'ativos', href.replace(/^ativos\//, ''))

    if (!figurasExistentes.has(caminhoImagem)) {
      pendencias.push({ capitulo: caminhoRelativo, alt, caminho: caminhoImagem })
      return (
        `<figure class="figura figura-pendente">` +
        `<div class="caixa-pendente">IMAGEM PENDENTE — ${escaparHtml(alt)}</div>` +
        `<figcaption>${rotulo} — ${escaparHtml(alt)}</figcaption></figure>\n`
      )
    }

    const url = pathToFileURL(caminhoImagem).href
    return (
      `<figure class="figura"><img src="${url}" alt="${escaparHtml(alt)}">` +
      `<figcaption>${rotulo} — ${escaparHtml(alt)}</figcaption></figure>\n`
    )
  }

  const html = marked.parse(markdown, { renderer })
  return { html, titulo, pendencias }
}

export async function montar(alvo, opcoes = {}) {
  const raiz = opcoes.raiz ?? RAIZ_PADRAO

  const manifesto = JSON.parse(await readFile(join(raiz, 'manifesto.json'), 'utf8'))
  const config = manifesto.alvos?.[alvo]
  if (!config) {
    throw new Error(`alvo desconhecido no manifesto: ${alvo}`)
  }

  const css = await readFile(join(raiz, 'ativos', 'css', 'manual.css'), 'utf8')

  const capitulos = []
  const pendencias = []
  let n = 0
  for (const caminho of config.capitulos) {
    n += 1
    const cap = await renderizarCapitulo(raiz, caminho, n)
    capitulos.push({ numero: n, titulo: cap.titulo, html: cap.html })
    pendencias.push(...cap.pendencias)
  }

  const sumario = capitulos
    .map((c) => `<li><a href="#cap-${c.numero}">${c.numero}. ${c.titulo}</a></li>`)
    .join('\n')

  const html = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<title>${escaparHtml(config.titulo)}</title>
<style>${css}</style>
</head>
<body>
<section class="capa">
  <h1 class="capa-titulo">${escaparHtml(config.titulo)}</h1>
  <p class="capa-subtitulo">${escaparHtml(config.subtitulo)}</p>
</section>
<nav id="sumario">
  <h2>Sumario</h2>
  <ol>
${sumario}
  </ol>
</nav>
<main id="conteudo">
${capitulos.map((c) => `<section class="capitulo">\n${c.html}</section>`).join('\n')}
</main>
</body>
</html>`

  return { html, pendencias }
}

// escreverChecklist grava CAPTURAS-PENDENTES.md com o que falta fotografar.
// O arquivo e sempre reescrito por inteiro: e derivado, nunca editado a mao.
export async function escreverChecklist(raiz, pendenciasPorAlvo) {
  const linhas = [
    '# Capturas pendentes',
    '',
    'Arquivo gerado por `npm run montar`. Nao editar a mao.',
    '',
    'Cada linha e uma figura que nenhum script consegue tirar — sao telas de',
    'aplicativo nativo (instalador do Docker Desktop, janelas do Windows).',
    'Tire a captura, salve com o nome exato indicado e rode o build de novo.',
    '',
    'Resolucao: 1440x900, PNG. Enquadre so a janela do aplicativo.',
    ''
  ]

  let total = 0
  for (const [alvo, pendencias] of Object.entries(pendenciasPorAlvo)) {
    if (pendencias.length === 0) continue
    linhas.push(`## ${alvo}`, '')
    for (const p of pendencias) {
      total += 1
      linhas.push(`- [ ] **${p.alt}**`)
      linhas.push(`  - capitulo: \`${p.capitulo}\``)
      linhas.push(`  - salvar em: \`${p.caminho}\``)
    }
    linhas.push('')
  }

  if (total === 0) {
    linhas.push('Nenhuma pendencia. Todas as figuras existem.', '')
  }

  await writeFile(join(raiz, 'CAPTURAS-PENDENTES.md'), linhas.join('\n'), 'utf8')
  return total
}

// Execucao pela linha de comando: monta os tres alvos e grava o checklist.
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const raiz = RAIZ_PADRAO
  const manifesto = JSON.parse(await readFile(join(raiz, 'manifesto.json'), 'utf8'))
  await mkdir(join(raiz, '.out'), { recursive: true })

  const pendenciasPorAlvo = {}
  for (const alvo of Object.keys(manifesto.alvos)) {
    const { html, pendencias } = await montar(alvo, { raiz })
    await writeFile(join(raiz, '.out', `${alvo}.html`), html, 'utf8')
    pendenciasPorAlvo[alvo] = pendencias
    console.log(`[manual] ${alvo}: montado`)
  }

  const total = await escreverChecklist(raiz, pendenciasPorAlvo)
  if (total > 0) {
    console.log(`[manual] ${total} imagens pendentes — veja CAPTURAS-PENDENTES.md`)
  }
}
```

- [ ] **Step 6: Escrever o CSS de impressão**

`docs/manual/ativos/css/manual.css`:

```css
/* Folha de impressao dos manuais. Pensada para papel A4 e para leitura
   por tecnico de campo: fonte grande, figura que nao quebra no meio. */

@page {
  size: A4;
  margin: 20mm 18mm 22mm 18mm;

  @bottom-center {
    content: counter(page);
    font-family: system-ui, sans-serif;
    font-size: 9pt;
    color: #666;
  }
}

body {
  font-family: system-ui, "Segoe UI", sans-serif;
  font-size: 11pt;
  line-height: 1.55;
  color: #1a1a1a;
}

.capa {
  break-after: page;
  padding-top: 60mm;
  text-align: center;
}
.capa-titulo { font-size: 26pt; margin: 0 0 8mm; }
.capa-subtitulo { font-size: 13pt; color: #555; }

#sumario { break-after: page; }
#sumario ol { list-style: none; padding-left: 0; }
#sumario li { margin: 2mm 0; font-size: 12pt; }
#sumario a { text-decoration: none; color: inherit; }

.capitulo { break-before: page; }
.capitulo h1 { font-size: 19pt; margin: 0 0 6mm; }
.capitulo h2 { font-size: 14pt; margin: 8mm 0 3mm; break-after: avoid; }

/* Figura nunca quebra entre paginas, e nunca fica separada da legenda. */
.figura {
  break-inside: avoid;
  margin: 6mm 0;
  text-align: center;
}
.figura img {
  max-width: 100%;
  border: 1px solid #ddd;
  border-radius: 3px;
}
.figura figcaption {
  font-size: 9.5pt;
  color: #555;
  margin-top: 2mm;
}

/* Caixa que ocupa o lugar de uma figura ainda nao capturada. Precisa ser
   visivelmente uma pendencia, nao passar por ilustracao. */
.caixa-pendente {
  border: 2px dashed #c00;
  color: #c00;
  padding: 18mm 6mm;
  font-weight: 600;
}

code, pre {
  font-family: "Cascadia Mono", Consolas, monospace;
  font-size: 9.5pt;
}
pre {
  background: #f6f6f6;
  border-left: 3px solid #999;
  padding: 3mm 4mm;
  break-inside: avoid;
  white-space: pre-wrap;
}

table { border-collapse: collapse; width: 100%; break-inside: avoid; }
th, td { border: 1px solid #ccc; padding: 2mm 3mm; text-align: left; font-size: 10pt; }
th { background: #f0f0f0; }
```

- [ ] **Step 7: Rodar os testes e confirmar que passam**

Run: `cd docs/manual && npm test`
Expected: PASS nos sete testes.

- [ ] **Step 8: Commit**

```bash
git add docs/manual/ .gitignore
git commit -m "$(cat <<'EOF'
feat(manual): assemble target HTML from shared markdown chapters

Primeira metade do pipeline dos manuais. Capítulos comuns são escritos uma
vez e referenciados pelos três alvos pelo manifesto; numeração de capítulo e
de figura é gerada, nunca escrita à mão.

Capítulo declarado e ausente do disco falha o build. Figura ausente não:
vira caixa tracejada e entra em CAPTURAS-PENDENTES.md. Algumas telas só um
humano consegue tirar, e se elas travassem o build o primeiro PDF nunca
sairia.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Impressão em PDF

**Files:**
- Create: `docs/manual/ferramentas/pdf.mjs`
- Test: `docs/manual/ferramentas/pdf.test.mjs`

**Interfaces:**
- Consumes: `docs/manual/.out/<alvo>.html` da Task 1.
- Produces: `imprimir(caminhoHtml, caminhoPdf) -> { paginas }`; `docs/manual/.out/<alvo>.pdf`.

- [ ] **Step 1: Escrever o teste que falha**

`docs/manual/ferramentas/pdf.test.mjs`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, writeFile, readFile, stat } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { imprimir } from './pdf.mjs'

// O teste imprime um HTML minimo de duas paginas. Nao depende do conteudo
// real dos manuais, que ainda esta sendo escrito.
test('imprime um PDF de verdade e conta as paginas', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'manual-pdf-'))
  const html = join(dir, 'teste.html')
  const pdf = join(dir, 'teste.pdf')

  await writeFile(
    html,
    `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8">
     <style>@page{size:A4;margin:20mm}.p{break-after:page}</style></head>
     <body><div class="p">Pagina um</div><div>Pagina dois</div></body></html>`,
    'utf8'
  )

  const { paginas } = await imprimir(html, pdf)

  assert.ok(paginas >= 2, `esperava ao menos 2 paginas, veio ${paginas}`)

  const bytes = await readFile(pdf)
  assert.equal(bytes.subarray(0, 4).toString('latin1'), '%PDF',
    'o arquivo precisa ser um PDF de verdade')

  const info = await stat(pdf)
  assert.ok(info.size > 1000, 'PDF suspeitosamente pequeno')
})
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `cd docs/manual && npm test`
Expected: FAIL — `Cannot find module './pdf.mjs'`.

- [ ] **Step 3: Escrever o `pdf.mjs`**

`docs/manual/ferramentas/pdf.mjs`:

```js
// Imprime o HTML montado em PDF, usando o mesmo Playwright que captura as
// telas — uma dependencia so para as duas coisas.
//
// A paginacao e feita pelo paged.js DENTRO da pagina, antes da impressao.
// Sem ele um PDF gerado por navegador nao tem numero de pagina nem sumario
// paginado, e manual impresso sem numero de pagina nao serve no campo.
import { readFile, mkdir } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createRequire } from 'node:module'

import { chromium } from 'playwright'

const AQUI = dirname(fileURLToPath(import.meta.url))
const RAIZ = resolve(AQUI, '..')
const require = createRequire(import.meta.url)

// O polyfill do paged.js vive dentro do pacote. Resolver pelo require evita
// chutar o caminho dentro de node_modules.
const PAGEDJS = require.resolve('pagedjs/dist/paged.polyfill.js')

export async function imprimir(caminhoHtml, caminhoPdf) {
  // channel: 'chrome' usa o Chrome ja instalado na maquina. Nunca rodamos
  // "playwright install" — os browsers proprios do Playwright sao 150 MB
  // que nao precisamos baixar.
  const browser = await chromium.launch({ channel: 'chrome' })
  try {
    const page = await browser.newPage()
    await page.goto(pathToFileURL(caminhoHtml).href, { waitUntil: 'load' })

    await page.addScriptTag({ path: PAGEDJS })
    // O paged.js avisa que terminou de paginar pelo atributo no <html>.
    await page.waitForFunction(
      () => document.documentElement.classList.contains('pagedjs_clearfix') ||
            document.querySelectorAll('.pagedjs_page').length > 0,
      null,
      { timeout: 120000 }
    )

    const paginas = await page.evaluate(
      () => document.querySelectorAll('.pagedjs_page').length
    )

    await page.pdf({
      path: caminhoPdf,
      format: 'A4',
      printBackground: true,
      // O paged.js ja desenhou margens e rodape dentro da pagina; deixar o
      // Chrome acrescentar as dele duplicaria tudo.
      margin: { top: '0', right: '0', bottom: '0', left: '0' },
      preferCSSPageSize: true
    })

    return { paginas }
  } finally {
    await browser.close()
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const manifesto = JSON.parse(await readFile(join(RAIZ, 'manifesto.json'), 'utf8'))
  await mkdir(join(RAIZ, '.out'), { recursive: true })

  for (const alvo of Object.keys(manifesto.alvos)) {
    const html = join(RAIZ, '.out', `${alvo}.html`)
    const pdf = join(RAIZ, '.out', `MANUAL-${alvo}.pdf`)
    const { paginas } = await imprimir(html, pdf)
    console.log(`[manual] ${alvo}: ${paginas} paginas -> ${pdf}`)
  }
}
```

- [ ] **Step 4: Rodar o teste e confirmar que passa**

Run: `cd docs/manual && npm test`
Expected: PASS. Se o Chrome não for encontrado, a mensagem do Playwright diz isso claramente — o Chrome está em `C:\Program Files\Google\Chrome\Application\chrome.exe` nesta máquina.

- [ ] **Step 5: Commit**

```bash
git add docs/manual/ferramentas/
git commit -m "$(cat <<'EOF'
feat(manual): print the assembled HTML to PDF

Mesmo Playwright que vai capturar as telas, então o pipeline inteiro tem uma
dependência de browser só, e ela usa o Chrome já instalado em vez de baixar
os 150 MB de browsers próprios.

O paged.js pagina dentro da página antes da impressão. Sem ele não há número
de página nem sumário paginado, e manual impresso sem número de página não
serve no campo. A contagem de páginas sai do DOM, não de um parser de PDF.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Captura das telas do emulador

**Files:**
- Create: `docs/manual/ferramentas/capturar.mjs`
- Create: `docs/manual/ferramentas/callout.mjs`
- Create: `docs/manual/.captura.env.exemplo`
- Test: `docs/manual/ferramentas/callout.test.mjs`

**Interfaces:**
- Consumes: nada das tarefas anteriores. Depende da marcação produzida pela Task 0 (`select.device-mode`, `#reachability-alert`).
- Produces: `cssDeCallout(marcadores) -> string` exportado por `callout.mjs`, e doze PNGs em `docs/manual/ativos/img/gerado/`: `emulador-console.png`, `emulador-rail.png`, `emulador-medidor.png`, `emulador-filtros.png`, `emulador-dispositivos.png`, `emulador-coluna-modo.png`, `emulador-gaveta-usuarios.png`, `emulador-gaveta-config.png`, `emulador-gaveta-log.png`, `emulador-aviso-portas.png` (condicional), `emulador-configuracoes.png` e `emulador-comparacao.png`. As Tasks 5 a 8 referenciam esses nomes.

**Pré-condições de ambiente.** Esta tarefa exige ambiente vivo. Antes do Step 1, confirme:

Run: `curl -s -o /dev/null -w "%{http_code}" http://localhost:7070/`
Expected: `200`.

Run: `curl -s http://localhost:7070/api/devices | head -c 400`
Expected: uma lista com vários dispositivos, incluindo **pelo menos um de modelo Dahua**. Sem nenhum Dahua, a captura `emulador-coluna-modo.png` falha no seletor `.device-mode` — corretamente, porque a figura não teria o que ilustrar. Se a rota de listagem tiver outro caminho, confirme em `internal/handlers/handlers.go`.

Se a aplicação não estiver de pé, suba-a antes: `docker compose up -d` na raiz do repositório, e aguarde o health check.

- [ ] **Step 1: Escrever o teste da função pura de callout**

O que dá para testar sem ambiente é a montagem do CSS e a validação dos seletores. `docs/manual/ferramentas/callout.test.mjs`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'

import { cssDeCallout, validarMarcadores } from './callout.mjs'

test('gera um circulo numerado por marcador, na ordem dada', () => {
  const css = cssDeCallout([
    { seletor: '#device-grid', numero: 1 },
    { seletor: '#filter-form', numero: 2 }
  ])

  assert.match(css, /#device-grid/)
  assert.match(css, /#filter-form/)
  assert.match(css, /content:\s*['"]1['"]/)
  assert.match(css, /content:\s*['"]2['"]/)
})

test('marcador sem seletor e recusado antes de tirar a foto', () => {
  assert.throws(() => validarMarcadores([{ numero: 1 }]), /seletor/)
})

test('numeros repetidos sao recusados', () => {
  assert.throws(
    () => validarMarcadores([
      { seletor: '#a', numero: 1 },
      { seletor: '#b', numero: 1 }
    ]),
    /repetido/
  )
})
```

- [ ] **Step 2: Rodar e confirmar que falha**

Run: `cd docs/manual && npm test`
Expected: FAIL — `Cannot find module './callout.mjs'`.

- [ ] **Step 3: Escrever o `callout.mjs`**

```js
// Callouts: circulos vermelhos numerados ancorados a elementos da pagina,
// injetados como CSS antes do disparo da foto.
//
// A alternativa seria editar PNG depois, o que envelhece mal: quando a tela
// muda, a seta continua apontando para o lugar antigo com cara de verdade.
// Aqui o seletor que nao casa quebra o script — ver capturar.mjs.

export function validarMarcadores(marcadores) {
  const vistos = new Set()
  for (const m of marcadores) {
    if (!m.seletor) {
      throw new Error(`marcador ${JSON.stringify(m)} sem seletor`)
    }
    if (typeof m.numero !== 'number') {
      throw new Error(`marcador ${m.seletor} sem numero`)
    }
    if (vistos.has(m.numero)) {
      throw new Error(`numero de callout repetido: ${m.numero}`)
    }
    vistos.add(m.numero)
  }
  return marcadores
}

export function cssDeCallout(marcadores) {
  validarMarcadores(marcadores)

  const regras = marcadores.map(
    (m) => `
${m.seletor} {
  position: relative !important;
  outline: 3px solid #d00 !important;
  outline-offset: 2px;
}
${m.seletor}::after {
  content: '${m.numero}';
  position: absolute;
  top: -14px;
  left: -14px;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: #d00;
  color: #fff;
  font: bold 16px/28px system-ui, sans-serif;
  text-align: center;
  z-index: 2147483647;
}`
  )

  return regras.join('\n')
}
```

- [ ] **Step 4: Rodar e confirmar que passa**

Run: `cd docs/manual && npm test`
Expected: PASS.

- [ ] **Step 5: Escrever o `capturar.mjs` com a suíte do emulador**

Todos os seletores abaixo foram lidos de `assets/web/templates/` na interface redesenhada. Não substitua nenhum por palpite: seletor que não casa derruba a captura, e é assim que este arquivo avisa que a tela mudou.

```js
// Captura as telas do emulador e do W-Access para os manuais.
//
// Passo manual e deliberado: exige ambiente vivo (banco, emulador, W-Access).
// O build dos PDFs NAO chama este script — ele monta a partir dos PNGs
// versionados, para que qualquer estacao gere release sem subir nada.
import { mkdir, readFile, access } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { chromium } from 'playwright'

import { cssDeCallout } from './callout.mjs'

const AQUI = dirname(fileURLToPath(import.meta.url))
const RAIZ = resolve(AQUI, '..')
const DESTINO = join(RAIZ, 'ativos', 'img', 'gerado')

const EMULADOR = process.env.EMULADOR_URL ?? 'http://localhost:7070'

// Resolucao fixa. deviceScaleFactor 2 porque em 1x a captura sai borrada
// no papel.
const VIEWPORT = { width: 1440, height: 900 }
const ESCALA = 2

// disparar tira a foto de uma pagina, com os callouts pedidos.
// Seletor que nao casa LANCA em vez de fotografar: uma seta apontando para
// o lugar errado e pior que figura nenhuma, e o erro precisa aparecer aqui
// e nao na ligacao do cliente.
async function disparar(page, { url, arquivo, marcadores = [], antes, alvo, pagina = true }) {
  if (url) await page.goto(url, { waitUntil: 'networkidle' })

  if (antes) await antes(page)

  for (const m of marcadores) {
    const encontrado = await page.$(m.seletor)
    if (!encontrado) {
      throw new Error(
        `seletor nao encontrado em ${page.url()}: ${m.seletor} — a tela mudou, ` +
        `atualize o marcador antes de gerar o manual`
      )
    }
  }

  if (marcadores.length > 0) {
    await page.addStyleTag({ content: cssDeCallout(marcadores) })
  }

  const caminho = join(DESTINO, arquivo)
  if (alvo) {
    // Recorte de um componente so. Uma pagina inteira de 1440x900 reduzida
    // para caber na coluna do PDF deixa o rail ilegivel.
    const elemento = await page.$(alvo)
    if (!elemento) throw new Error(`alvo de recorte nao encontrado: ${alvo}`)
    await elemento.screenshot({ path: caminho })
  } else {
    await page.screenshot({ path: caminho, fullPage: pagina })
  }
  console.log(`[captura] ${arquivo}`)
}

async function suiteEmulador(browser) {
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: ESCALA
  })
  const page = await context.newPage()

  // 1. A tela inteira, com os tres pontos de referencia do capitulo de tour.
  await disparar(page, {
    url: `${EMULADOR}/`,
    arquivo: 'emulador-console.png',
    marcadores: [
      { seletor: '#rail', numero: 1 },
      { seletor: '#fleet-meter', numero: 2 },
      { seletor: '#device-grid', numero: 3 }
    ]
  })

  // 2. O rail recortado, com o sinal de conexao do W-Access.
  await disparar(page, {
    url: `${EMULADOR}/`,
    arquivo: 'emulador-rail.png',
    alvo: '#rail',
    marcadores: [
      { seletor: '[data-nav="/"]', numero: 1 },
      { seletor: '[data-nav="/comparison"]', numero: 2 },
      { seletor: '[data-nav="/settings"]', numero: 3 },
      { seletor: '#start-all', numero: 4 },
      { seletor: '#sync-db', numero: 5 }
    ]
  })

  // 3. O medidor de frota recortado.
  await disparar(page, {
    url: `${EMULADOR}/`,
    arquivo: 'emulador-medidor.png',
    alvo: '#fleet-meter',
    marcadores: [
      { seletor: '#meter-bar', numero: 1 },
      { seletor: '#meter-reading', numero: 2 },
      { seletor: '#meter-health', numero: 3 }
    ]
  })

  // 4. Os filtros e a paginacao.
  await disparar(page, {
    url: `${EMULADOR}/`,
    arquivo: 'emulador-filtros.png',
    alvo: '#filter-form',
    marcadores: [
      { seletor: '#filter-id', numero: 1 },
      { seletor: '#filter-name', numero: 2 },
      { seletor: '#filter-port', numero: 3 }
    ]
  })

  // 5. A grade, com a selecao em lote e a coluna Modo.
  await disparar(page, {
    url: `${EMULADOR}/`,
    arquivo: 'emulador-dispositivos.png',
    alvo: '#device-grid',
    marcadores: [
      { seletor: '#select-all', numero: 1 },
      { seletor: '.device-mode', numero: 2 },
      { seletor: '.row-actions', numero: 3 }
    ]
  })

  // 6. A coluna Modo em detalhe. O seletor so existe para dispositivos
  // Dahua; sem nenhum Dahua na frota, .device-mode nao casa e a captura
  // falha aqui — o que e o aviso correto, nao um bug.
  await disparar(page, {
    url: `${EMULADOR}/`,
    arquivo: 'emulador-coluna-modo.png',
    alvo: '#device-grid',
    marcadores: [{ seletor: '.device-mode', numero: 1 }]
  })

  // 7. A gaveta de detalhes, aba de usuarios. Abre pelo botao de detalhes
  // da primeira linha.
  await disparar(page, {
    url: `${EMULADOR}/`,
    arquivo: 'emulador-gaveta-usuarios.png',
    antes: async (p) => {
      await p.click('.device-details-btn')
      await p.waitForSelector('#device-drawer:not([hidden])')
    },
    marcadores: [
      { seletor: '#drawer-title', numero: 1 },
      { seletor: '#drawer-led', numero: 2 },
      { seletor: '#tab-users', numero: 3 },
      { seletor: '#users-search', numero: 4 }
    ]
  })

  // 8. A mesma gaveta, aba de configuracoes.
  await disparar(page, {
    url: null,
    arquivo: 'emulador-gaveta-config.png',
    antes: async (p) => {
      await p.click('#tab-settings')
      await p.waitForSelector('#panel-settings:not([hidden])')
    },
    marcadores: [{ seletor: '#panel-settings', numero: 1 }]
  })

  // 9. O log do dispositivo, com o botao de salvar.
  await disparar(page, {
    url: null,
    arquivo: 'emulador-gaveta-log.png',
    marcadores: [
      { seletor: '#drawer-log', numero: 1 },
      { seletor: '#drawer-save-log', numero: 2 }
    ],
    antes: async (p) => {
      await p.waitForSelector('#drawer-log')
    }
  })

  // Fecha a gaveta antes de sair da pagina.
  await page.click('#drawer-close')

  // 10. O aviso de alcancabilidade so aparece quando ha o que avisar. Se o
  // ambiente estiver saudavel, ele fica escondido — nesse caso a figura
  // continua pendente em vez de sair uma foto de tela vazia.
  await page.goto(`${EMULADOR}/`, { waitUntil: 'networkidle' })
  const avisoVisivel = await page.isVisible('#reachability-alert')
  if (avisoVisivel) {
    await disparar(page, {
      url: null,
      arquivo: 'emulador-aviso-portas.png',
      alvo: '#reachability-alert',
      marcadores: [
        { seletor: '#reachability-headline', numero: 1 },
        { seletor: '#reachability-toggle', numero: 2 }
      ]
    })
  } else {
    console.log(
      '[captura] aviso de alcancabilidade nao esta visivel neste ambiente — ' +
      'emulador-aviso-portas.png continua pendente'
    )
  }

  // 11. A tela de configuracoes do W-Access.
  await disparar(page, {
    url: `${EMULADOR}/settings`,
    arquivo: 'emulador-configuracoes.png',
    marcadores: [
      { seletor: '#wxs-host', numero: 1 },
      { seletor: '#wxs-database', numero: 2 },
      { seletor: '#test-connection', numero: 3 }
    ],
    // Nenhuma figura carrega o que esta na maquina de quem gerou: os campos
    // sao preenchidos com valores de exemplo e a senha fica mascarada.
    antes: async (p) => {
      await p.fill('#wxs-host', 'servidor-wxs')
      await p.fill('#wxs-port', '1433')
      await p.fill('#wxs-database', 'W_Access')
      await p.fill('#wxs-username', 'usuario')
      await p.fill('#wxs-password', '••••••••')
    }
  })

  // 12. A pagina de comparacao.
  await disparar(page, {
    url: `${EMULADOR}/comparison`,
    arquivo: 'emulador-comparacao.png',
    marcadores: [
      { seletor: '#comparison-grid', numero: 1 },
      { seletor: '#refresh-comparison', numero: 2 }
    ]
  })

  await context.close()
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await mkdir(DESTINO, { recursive: true })
  const browser = await chromium.launch({ channel: 'chrome' })
  try {
    await suiteEmulador(browser)
  } finally {
    await browser.close()
  }
}
```

Dois pontos que costumam morder na primeira execução:

- `#device-drawer:not([hidden])` assume que a gaveta usa o atributo `hidden` para esconder. Confirme em `assets/web/static/js/device-drawer.js` antes de rodar; se ela usar classe, troque o seletor de espera pela classe real.
- As capturas 8 e 9 dependem do estado deixado pela 7 (gaveta aberta) e por isso passam `url: null`. Não reordene.

- [ ] **Step 6: Escrever o arquivo de exemplo de credenciais**

`docs/manual/.captura.env.exemplo`:

```
# Copie para .captura.env e preencha. O .captura.env NAO vai para o git.
# Sem ele, a suite do W-Access e pulada e a do emulador roda normalmente.
WXS_URL=https://localhost/W-Access
WXS_USUARIO=
WXS_SENHA=
# URL do emulador, se nao for o padrao
EMULADOR_URL=http://localhost:7070
```

- [ ] **Step 7: Verificar**

Run: `cd docs/manual && node --check ferramentas/capturar.mjs && npm test`
Expected: sem saída do `node --check`, PASS nos testes.

Se o emulador estiver rodando em `localhost:7070`, rodar `npm run capturar` e conferir os PNGs em `ativos/img/gerado/`. Se não estiver, registrar a captura como pendente no relatório e seguir — nenhuma tarefa posterior depende dos PNGs existirem.

- [ ] **Step 8: Commit**

```bash
git add docs/manual/
git commit -m "$(cat <<'EOF'
feat(manual): capture the emulator screens with callouts

Callout injetado como CSS antes do disparo, não desenhado sobre o PNG
depois: quando a tela muda, seletor que não casa quebra o script em vez de
gerar uma seta apontando para o lugar errado com cara de verdade.

A tela de configurações é fotografada com valores de exemplo e senha
mascarada — nenhuma figura carrega o que está na máquina de quem gerou.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Captura das telas do W-Access

**Files:**
- Modify: `docs/manual/ferramentas/capturar.mjs`

**Interfaces:**
- Consumes: `disparar` e `cssDeCallout` da Task 3.
- Produces: `docs/manual/ativos/img/gerado/wxs-login.png`, `wxs-pos-login.png`, `wxs-controladores.png` e `wxs-inicial.png` — este último é o cadastro do controlador, referenciado pelo capítulo `configurar-wxs.md` da Task 5.

- [ ] **Step 1: Acrescentar a leitura do arquivo de credenciais**

Em `capturar.mjs`, antes do bloco de execução por linha de comando:

```js
// lerCredenciais le .captura.env. Formato: uma linha CHAVE=valor por vez.
// Ausente ou incompleto, a suite do W-Access e pulada com aviso — nunca
// falha, porque a suite do emulador continua util sozinha.
async function lerCredenciais() {
  const caminho = join(RAIZ, '.captura.env')
  try {
    await access(caminho)
  } catch {
    return null
  }

  const env = {}
  for (const linha of (await readFile(caminho, 'utf8')).split(/\r?\n/)) {
    const limpa = linha.trim()
    if (!limpa || limpa.startsWith('#')) continue
    const i = limpa.indexOf('=')
    if (i < 0) continue
    env[limpa.slice(0, i).trim()] = limpa.slice(i + 1).trim()
  }

  if (!env.WXS_URL || !env.WXS_USUARIO || !env.WXS_SENHA) return null
  return env
}
```

- [ ] **Step 2: Escrever a suíte do W-Access**

Ainda em `capturar.mjs`:

```js
// O W-Access roda em HTTPS com certificado self-signed, dai o
// ignoreHTTPSErrors. E um ASP.NET WebForms: a tela de login e Login.aspx e
// os ids dos campos precisam ser confirmados na primeira execucao.
async function suiteWxs(browser, env) {
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: ESCALA,
    ignoreHTTPSErrors: true
  })
  const page = await context.newPage()

  await page.goto(env.WXS_URL, { waitUntil: 'networkidle' })

  // A tela de login e fotografada ANTES de preencher, para o manual mostrar
  // o campo vazio e nao a credencial de quem gerou.
  await disparar(page, {
    url: page.url(),
    arquivo: 'wxs-login.png'
  })

  await page.fill('input[type="text"]', env.WXS_USUARIO)
  await page.fill('input[type="password"]', env.WXS_SENHA)
  await page.keyboard.press('Enter')
  await page.waitForLoadState('networkidle')

  // A tela inicial depois do login, so para o manual mostrar que o acesso
  // deu certo antes de mandar navegar.
  await disparar(page, {
    url: page.url(),
    arquivo: 'wxs-pos-login.png'
  })

  // Tela de controladores. A navegacao e por texto de link porque os ids
  // gerados pelo WebForms nao sao estaveis entre versoes do W-Access.
  // Se o texto do menu for outro nesta instalacao, ajuste as duas strings
  // abaixo — e so isso que muda.
  await page.getByRole('link', { name: /dispositivos|devices/i }).first().click()
  await page.getByRole('link', { name: /controladores|controllers/i }).first().click()
  await page.waitForLoadState('networkidle')

  await disparar(page, {
    url: null,
    arquivo: 'wxs-controladores.png'
  })

  // Cadastro de um controlador: e aqui que a descricao emulator_NN, o
  // endereco e o BaseCommPort sao preenchidos. Abre o primeiro da lista.
  // Este e o wxs-inicial.png que o capitulo de configuracao referencia.
  await page.locator('table a').first().click()
  await page.waitForLoadState('networkidle')

  await disparar(page, {
    url: null,
    arquivo: 'wxs-inicial.png'
  })

  await context.close()
}
```

E no bloco de execução por linha de comando, depois de `await suiteEmulador(browser)`:

```js
    const env = await lerCredenciais()
    if (env) {
      await suiteWxs(browser, env)
    } else {
      console.log(
        '[captura] .captura.env ausente ou incompleto — suite do W-Access ' +
        'pulada. Copie .captura.env.exemplo e preencha.'
      )
    }
```

- [ ] **Step 3: Verificar**

Run: `cd docs/manual && node --check ferramentas/capturar.mjs && npm test`
Expected: sem saída, PASS.

Com o W-Access acessível e o `.captura.env` preenchido:

Run: `cd docs/manual && npm run capturar`
Expected: `wxs-login.png`, `wxs-pos-login.png`, `wxs-controladores.png` e `wxs-inicial.png` em `ativos/img/gerado/`.

Três pontos deste script são chute informado sobre um ASP.NET WebForms que ainda não foi aberto, e **têm de ser confirmados nesta primeira execução real**: os seletores de login (`input[type="text"]`, `input[type="password"]`), os textos dos links de menu (`dispositivos`/`controladores`), e o `table a` que abre o primeiro controlador. Quando algum não casar, o Playwright falha apontando qual — ajuste com o valor verdadeiro lido da página e rode de novo. Não troque a falha por um `try/catch`: seletor errado tem de derrubar a captura.

Se o W-Access não estiver alcançável, as três figuras ficam pendentes, a suíte do emulador roda até o fim, e o PDF sai com caixa tracejada no lugar delas. Registrar no relatório e seguir.

- [ ] **Step 4: Commit**

```bash
git add docs/manual/ferramentas/capturar.mjs
git commit -m "$(cat <<'EOF'
feat(manual): capture the W-Access screens

O capítulo de configuração precisa mostrar onde o controlador é cadastrado
no W-Access, não descrever de memória.

O login sai de .captura.env, que é gitignored — três commits recentes
tiraram credencial de arquivo versionado e nada aqui as traz de volta pela
porta dos fundos. Sem o arquivo, a suíte é pulada e a do emulador roda.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Capítulos comuns aos três manuais

Cinco capítulos escritos uma vez, referenciados pelos três alvos.

**Files:**
- Create: `docs/manual/conteudo/comum/conhecer-o-console.md`
- Create: `docs/manual/conteudo/comum/configurar-wxs.md`
- Create: `docs/manual/conteudo/comum/validacao.md`
- Create: `docs/manual/conteudo/comum/logs.md`
- Create: `docs/manual/conteudo/comum/problemas.md`

**Interfaces:**
- Consumes: o formato de capítulo da Task 1 (um `#` de título por arquivo, figuras por `![alt](img/...)`) e os PNGs produzidos pelas Tasks 3 e 4.
- Produces: cinco capítulos referenciáveis pelo manifesto nas Tasks 6, 7 e 8.

- [ ] **Step 1: Escrever `conhecer-o-console.md`**

Título `# Conhecer o console`. É o capítulo que o plano anterior não tinha: a interface foi redesenhada inteira, e sem este capítulo o técnico instala e fica olhando para uma tela que ninguém explicou. Cobrir, nesta ordem, os sete blocos da seção 11.4 da spec:

1. **A barra lateral e a navegação.** As três páginas — Dispositivos, Comparação e Conexão W-Access — e o botão que expande a barra para mostrar os nomes. Depois, as três ações de frota: iniciar todos, parar todos, sincronizar com o W-Access. Figura `![A barra lateral](img/gerado/emulador-rail.png)`.
2. **O sinal de conexão do W-Access.** Um ponto vermelho aparece ao lado de "Conexão W-Access" quando as credenciais gravadas não conectam. Diga o que fazer quando ele acende: abrir a página de conexão, conferir servidor, banco, usuário e senha, e testar. Sem o ponto, a conexão está boa.
3. **O medidor de frota.** A barra no topo, um segmento por emulador, com a leitura ao lado. Explique os três estados — ativo, parado, desabilitado — e que a barra se atualiza sozinha. Figura `![O medidor de frota](img/gerado/emulador-medidor.png)`.
4. **A lista de dispositivos.** As colunas, uma frase por coluna. Na coluna **Modo**, explique os dois valores em uma frase cada — online, o Site Controller valida o acesso e responde; standalone, o dispositivo valida sozinho e gera o evento — e que a troca vale na hora. Diga também por que algumas linhas mostram um traço em vez do seletor: só os dispositivos Dahua usam essa configuração. Depois, a caixa de seleção do cabeçalho e os botões de iniciar e parar os selecionados. Figuras `![A lista de dispositivos](img/gerado/emulador-dispositivos.png)` e `![A coluna Modo](img/gerado/emulador-coluna-modo.png)`.
5. **Filtrar e paginar.** Os três campos — identificador, nome e porta — o botão de limpar, e a escolha de quantos itens por página. Figura `![Os filtros](img/gerado/emulador-filtros.png)`.
6. **A gaveta de detalhes.** Como abrir pelo botão da linha; o nome e a luz de estado no topo; a aba de usuários com busca e paginação; a aba de configurações; e o log do dispositivo, com o botão que salva o log em arquivo. Figuras `![A gaveta, aba de usuarios](img/gerado/emulador-gaveta-usuarios.png)`, `![A gaveta, aba de configuracoes](img/gerado/emulador-gaveta-config.png)` e `![O log do dispositivo](img/gerado/emulador-gaveta-log.png)`.
7. **A tela se atualiza sozinha.** Uma seção curta: o estado dos dispositivos e o medidor mudam sem precisar recarregar, e é por isso que quase nenhuma tela tem botão de atualizar. Se a tela parar de mudar, recarregue a página — a conexão se refaz sozinha.
8. **A página de comparação.** Para que serve e como ler o resultado. Figura `![A pagina de comparacao](img/gerado/emulador-comparacao.png)`.

Abrir o capítulo com a figura da tela inteira: `![O console do emulador](img/gerado/emulador-console.png)`.

Restrição que vale especialmente aqui: nada de `SSE`, `template`, `endpoint` ou `handler` no texto. O técnico precisa saber o que vê e o que fazer, não como foi construído.

- [ ] **Step 2: Escrever `configurar-wxs.md`**

Título `# Configurar o W-Access`. Cobrir, nesta ordem, em português para técnico de campo:

1. O que precisa existir no W-Access para o emulador enxergar um controlador: a descrição do controlador tem de **começar com `emulator`** — é assim que o emulador filtra (`LocalControllerDescription LIKE 'emulator%'`). Endereço e `BaseCommPort` são o IP e a porta que o emulador vai abrir.
2. Figura `![Cadastro do controlador no W-Access](img/gerado/wxs-inicial.png)`.
3. A tela de conexão do emulador: servidor, porta 1433, banco `W_Access`, usuário e senha; o botão que mostra a senha digitada; o botão que testa a conexão. Figura `![Tela de conexao com o W-Access](img/gerado/emulador-configuracoes.png)`.
4. Testar a conexão e salvar; voltar à lista de dispositivos e sincronizar. Aponte para o capítulo Conhecer o console para o significado do ponto vermelho no menu.

O modo online e standalone **não** é explicado aqui — está no capítulo Conhecer o console, item 4. Referencie-o em vez de repetir.

- [ ] **Step 3: Escrever `validacao.md`**

Título `# Roteiro de validacao`. Passo a passo numerado, cada passo com **o que fazer** e **o que precisa acontecer**, e para onde ir quando não acontecer. Mínimo:

1. Abrir `http://localhost:7070` — a lista de dispositivos carrega e mostra os controladores marcados `emulator` no W-Access. Se vier vazia: capítulo Problemas, "a lista de dispositivos está vazia".
2. Clicar em iniciar — o estado dos dispositivos vira `ativo` e o medidor de frota acompanha. Se algum não subir: capítulo Problemas, "um dispositivo não inicia".
3. Conferir no W-Access que os controladores aparecem online. Se aparecerem offline com a tela do emulador toda verde: capítulo Portas e rede — é o caso mais comum e o mais confuso.
4. Simular uma leitura facial pelo emulador e conferir o evento chegando no W-Access.
5. Conferir a linha correspondente no log do dispositivo, pela gaveta de detalhes, e no log da aplicação, com o caminho apontando para o capítulo Logs.

Cada passo carrega uma figura da tela correspondente quando ela existir; onde ainda não houver captura, referenciar o nome do arquivo esperado em `img/gerado/` para que o placeholder registre a pendência.

- [ ] **Step 4: Escrever `logs.md`**

Título `# Onde estao os logs`. Uma tabela com uma linha por pacote apontando o caminho, mais a explicação de que `trace.html` é a versão colorida do `trace.log` e abre no navegador, e de que o log de um dispositivo específico sai pela gaveta de detalhes, no botão que salva o log em arquivo. Os caminhos por pacote:

| Pacote | Aplicação | Banco | Instalação |
|---|---|---|---|
| Docker | `sistema/logs/trace.log` | `docker compose -f sistema/docker-compose.yml logs postgres` | `sistema/logs/instalacao.log` |
| Windows | `sistema\logs\trace.log` | `sistema\logs\postgres.log` | `sistema\logs\instalacao.log` |
| Linux/WSL | `sistema/logs/trace.log` e `sistema/logs/app.out` | `/var/log/postgresql/` | `sistema/logs/instalacao.log` |

- [ ] **Step 5: Escrever `problemas.md`**

Título `# Problemas comuns`. Tabela sintoma → causa → o que fazer, cobrindo no mínimo:

- A lista de dispositivos está vazia → a conexão com o W-Access falhou, ou nenhum controlador tem descrição começando com `emulator`.
- Ponto vermelho ao lado de "Conexão W-Access" no menu → as credenciais gravadas não conectam; abrir a página de conexão e testar.
- Tudo verde na tela e tudo offline no W-Access → a porta do controlador não é alcançável neste ambiente; ler o aviso no topo da lista e o capítulo Portas e rede.
- Um dispositivo não inicia → a porta já está em uso por outro processo; o log da aplicação traz o erro do sistema operacional.
- A coluna Modo mostra um traço → o dispositivo não é Dahua; só esse modelo usa a configuração.
- A tela parou de se atualizar sozinha → recarregar a página.
- `INICIAR` diz que a aplicação não respondeu em 60 segundos → ver `trace.log`; no Docker, `docker compose logs app`.
- A página abre sem formatação → não deve acontecer; se acontecer, o pacote está incompleto, baixar de novo.

- [ ] **Step 6: Verificar que os capítulos são montáveis**

Ainda não estão no manifesto, então crie um alvo temporário para testar, ou confirme na Task 6 quando o primeiro alvo real for montado. O que precisa valer agora: cada arquivo tem exatamente um `#` de título na primeira linha, e nenhum número de capítulo escrito à mão.

Run: `cd docs/manual && grep -c "^# " conteudo/comum/*.md`
Expected: `1` para cada um dos cinco arquivos.

- [ ] **Step 7: Commit**

```bash
git add docs/manual/conteudo/comum/
git commit -m "$(cat <<'EOF'
docs(manual): write the five shared chapters

Tour do console, configuração do W-Access, roteiro de validação, logs e
problemas comuns — escritos uma vez, referenciados pelos três manuais.

O tour é capítulo novo: a interface foi redesenhada inteira, e sem ele o
técnico instala e fica olhando para uma tela que ninguém explicou.

O roteiro de validação é o capítulo que responde "funcionou?": cada passo
traz o que fazer, o que precisa acontecer, e para onde ir quando não
acontecer.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Manual do pacote Docker

**Files:**
- Create: `docs/manual/conteudo/docker/antes-de-comecar.md`
- Create: `docs/manual/conteudo/docker/instalar-docker-desktop.md`
- Create: `docs/manual/conteudo/docker/instalar-emulador.md`
- Create: `docs/manual/conteudo/docker/portas-e-rede.md`
- Modify: `docs/manual/manifesto.json`

**Interfaces:**
- Consumes: os capítulos comuns da Task 5.
- Produces: o alvo `docker` completo no manifesto; o primeiro PDF real.

- [ ] **Step 1: Escrever `antes-de-comecar.md`**

Título `# Antes de comecar`. O que o técnico precisa ter em mãos: o ZIP `GoFacialEmulator-docker.zip`, uma máquina com Windows 10 ou 11, permissão de administrador para instalar o Docker Desktop, e os dados de acesso ao banco do W-Access. Onde extrair: `C:\GoFacialEmulator`, evitando Área de Trabalho, Documentos e OneDrive, porque a sincronização atrapalha o banco de dados.

- [ ] **Step 2: Escrever `instalar-docker-desktop.md`**

Título `# Instalar o Docker Desktop`. Este é o capítulo mais ilustrado e o que mais depende de captura humana. Escrever o passo a passo em texto completo e referenciar as figuras pelos nomes abaixo — todas em `img/manual/`, todas pendentes até alguém tirá-las:

- `![Pagina de download do Docker Desktop](img/manual/docker-download.png)`
- `![Aviso de configuracao do instalador do Docker Desktop](img/manual/docker-wizard-01.png)`
- `![Instalacao em andamento](img/manual/docker-wizard-02.png)`
- `![Instalacao concluida, pedindo reinicio](img/manual/docker-wizard-03.png)`
- `![Termos de uso do Docker Desktop](img/manual/docker-termos.png)`
- `![Docker Desktop aberto com o icone verde](img/manual/docker-verde.png)`

O texto precisa deixar claro: baixar em `https://www.docker.com/products/docker-desktop/`, aceitar a opção do WSL2 quando o instalador oferecer, reiniciar quando pedido, abrir o Docker Desktop e **esperar o indicador ficar verde** antes de seguir — é aí que a maioria erra, rodando o `INSTALAR.bat` com o Docker ainda subindo.

- [ ] **Step 3: Escrever `instalar-emulador.md`**

Título `# Instalar o emulador`. Extrair o ZIP, duplo-clique em `INSTALAR.bat` (só na primeira vez), esperar `✅ Instalado`, duplo-clique em `INICIAR.bat`, esperar `✅ Rodando em http://localhost:7070`, abrir o navegador. Para parar: `PARAR.bat`. Em Linux, `./instalar.sh`, `./iniciar.sh`, `./parar.sh`.

- [ ] **Step 4: Escrever `portas-e-rede.md`**

Título `# Portas e rede`. Conteúdo específico do pacote Docker:

- As portas dos emuladores vêm do W-Access, do campo `BaseCommPort` de cada controlador — o emulador obedece, não escolhe.
- No Windows este pacote publica a faixa **4000-4499**. Controlador com porta fora dessa faixa sobe, escuta dentro do container, e **não é alcançado** pelo Site Controller. A tela avisa quando isso acontece — figura `![Aviso de portas nao publicadas](img/gerado/emulador-aviso-portas.png)`.
- Como alargar: editar `sistema\docker-compose.yml`; a linha `ports` e a variável `PUBLISHED_PORT_RANGE` precisam ter **a mesma faixa**. Mostrar o trecho do arquivo.
- Em Linux não há limite: o pacote usa a rede do host, e qualquer porta que o W-Access pedir funciona.

- [ ] **Step 5: Preencher o alvo `docker` no manifesto**

Em `docs/manual/manifesto.json`, a lista `capitulos` do alvo `docker`:

```json
      "capitulos": [
        "docker/antes-de-comecar.md",
        "docker/instalar-docker-desktop.md",
        "docker/instalar-emulador.md",
        "comum/conhecer-o-console.md",
        "comum/configurar-wxs.md",
        "comum/validacao.md",
        "docker/portas-e-rede.md",
        "comum/logs.md",
        "comum/problemas.md"
      ]
```

- [ ] **Step 6: Montar e imprimir**

Run: `cd docs/manual && npm run manuais`
Expected: `[manual] docker: montado`, a contagem de imagens pendentes, e `[manual] docker: N paginas -> .out/MANUAL-docker.pdf` com N de pelo menos 10.

Run: `cd docs/manual && cat CAPTURAS-PENDENTES.md`
Expected: as seis figuras do capítulo do Docker Desktop listadas com o caminho exato onde salvar.

Abrir `.out/MANUAL-docker.pdf` e conferir: capa, sumário com números de página, capítulos numerados em ordem, e as caixas tracejadas vermelhas no lugar das figuras que faltam.

- [ ] **Step 7: Commit**

```bash
git add docs/manual/
git commit -m "$(cat <<'EOF'
docs(manual): write the Docker package manual

Primeiro alvo completo, e o que valida o pipeline de ponta a ponta.

O capítulo do Docker Desktop é o mais ilustrado e o que mais depende de
captura humana: seis telas de instalador nativo que nenhum script alcança.
Elas saem como caixa tracejada e entram em CAPTURAS-PENDENTES.md, então o
manual já é utilizável enquanto a dívida fica visível.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Manual do pacote Windows

**Files:**
- Create: `docs/manual/conteudo/windows/antes-de-comecar.md`
- Create: `docs/manual/conteudo/windows/instalar-emulador.md`
- Create: `docs/manual/conteudo/windows/portas-e-rede.md`
- Modify: `docs/manual/manifesto.json`

**Interfaces:**
- Consumes: os capítulos comuns da Task 5.
- Produces: o alvo `windows` completo.

- [ ] **Step 1: Escrever `antes-de-comecar.md`**

Título `# Antes de comecar`. O ZIP `GoFacialEmulator-windows.zip`, Windows 10 ou 11, **nenhuma outra instalação necessária — o banco de dados já vem junto**, e os dados de acesso ao W-Access. Extrair em `C:\GoFacialEmulator`, longe de OneDrive e Documentos.

- [ ] **Step 2: Escrever `instalar-emulador.md`**

Título `# Instalar o emulador`. Duplo-clique em `INSTALAR.bat`, que demora cerca de um minuto porque cria o banco de dados embutido; esperar `✅ Instalado`. Depois `INICIAR.bat` e esperar `✅ Rodando em http://localhost:7070`. Rodar `INSTALAR.bat` de novo não faz mal: ele detecta que o banco já existe e não apaga nada. Para parar, `PARAR.bat`, que encerra a aplicação **e** o banco, inclusive quando a janela da aplicação já foi fechada.

Figuras esperadas, todas em `img/manual/` e pendentes: `![Pasta extraida em C:\GoFacialEmulator](img/manual/windows-pasta.png)` e `![Janela do INSTALAR.bat concluida](img/manual/windows-instalar.png)`.

- [ ] **Step 3: Escrever `portas-e-rede.md`**

Título `# Portas e rede`. Específico do pacote Windows:

- As portas vêm do W-Access; o emulador obedece.
- Não há limite de faixa publicada neste pacote: a aplicação roda direto no Windows e abre as portas na máquina.
- O que pode bloquear é o **Firewall do Windows**. Quando o Site Controller não conecta e a tela do emulador está verde, liberar no firewall a faixa de portas usada pelos controladores é a primeira coisa a checar. Descrever o caminho: Firewall do Windows Defender → Configurações avançadas → Regras de entrada → Nova regra → Porta → TCP → faixa.
- Porta já em uso por outro processo aparece no log da aplicação com o erro do sistema operacional, e o dispositivo aparece como inalcançável na tela.

- [ ] **Step 4: Preencher o alvo `windows` no manifesto**

```json
      "capitulos": [
        "windows/antes-de-comecar.md",
        "windows/instalar-emulador.md",
        "comum/conhecer-o-console.md",
        "comum/configurar-wxs.md",
        "comum/validacao.md",
        "windows/portas-e-rede.md",
        "comum/logs.md",
        "comum/problemas.md"
      ]
```

- [ ] **Step 5: Montar, imprimir e conferir**

Run: `cd docs/manual && npm run manuais`
Expected: os dois alvos montam, `MANUAL-windows.pdf` sai com pelo menos 10 páginas.

Conferir no PDF que **nada sobre Docker aparece** — é o ponto de ter um manual por pacote.

- [ ] **Step 6: Commit**

```bash
git add docs/manual/
git commit -m "$(cat <<'EOF'
docs(manual): write the Windows package manual

Nenhuma menção a Docker: quem pegou este pacote não precisa saber que Docker
existe, e a trilha errada é onde o usuário leigo erra.

O capítulo de portas troca o assunto do publish do Docker pelo Firewall do
Windows, que é o que de fato bloqueia neste pacote.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Manual do pacote Linux e WSL2

**Files:**
- Create: `docs/manual/conteudo/linux/antes-de-comecar.md`
- Create: `docs/manual/conteudo/linux/instalar-wsl.md`
- Create: `docs/manual/conteudo/linux/instalar-emulador.md`
- Create: `docs/manual/conteudo/linux/portas-e-rede.md`
- Modify: `docs/manual/manifesto.json`

**Interfaces:**
- Consumes: os capítulos comuns da Task 5.
- Produces: o alvo `linux` completo; os três manuais existindo.

- [ ] **Step 1: Escrever `antes-de-comecar.md`**

Título `# Antes de comecar`. Dois públicos no mesmo manual: servidor Linux, e Windows rodando WSL2. Dizer logo no começo qual seção pular. O ZIP `GoFacialEmulator-linux.zip`, acesso `sudo`, e os dados do W-Access. Extrair em `/opt/gofacialemulator` ou na home do usuário.

- [ ] **Step 2: Escrever `instalar-wsl.md`**

Título `# Instalar o WSL2`. Só para quem está no Windows; a primeira linha diz para pular se o destino é um servidor Linux.

Passo a passo: abrir o PowerShell **como administrador**, rodar `wsl --install`, reiniciar quando pedido, e na primeira abertura do Ubuntu criar usuário e senha.

Figuras esperadas, em `img/manual/`, pendentes: `![PowerShell como administrador](img/manual/wsl-powershell-admin.png)`, `![Saida do comando wsl --install](img/manual/wsl-install.png)`, `![Primeira abertura do Ubuntu pedindo usuario](img/manual/wsl-primeiro-uso.png)`.

Fechar o capítulo com o aviso que mais gera chamado, em destaque: **no WSL2, por padrão, só a própria máquina alcança os emuladores.** O Site Controller em outro computador não conecta. Mostrar o bloco a colar em `C:\Users\SEU_USUARIO\.wslconfig`:

```ini
[wsl2]
networkingMode=mirrored
```

e depois `wsl --shutdown` no PowerShell. Dizer que o `instalar.sh` detecta e avisa quando isso está faltando.

- [ ] **Step 3: Escrever `instalar-emulador.md`**

Título `# Instalar o emulador`. `sudo bash instalar.sh`, que pergunta a faixa de portas com padrão `4000-4499` — explicar que a faixa deve cobrir os `BaseCommPort` dos controladores no W-Access. O instalador instala o PostgreSQL, cria o banco, libera o firewall e ajusta o limite de arquivos abertos. Depois `./iniciar.sh` e `./parar.sh`. Se os scripts não estiverem executáveis, porque o ZIP não preserva essa permissão, usar `bash iniciar.sh`.

Mencionar que, se o instalador disser que ajustou o limite de arquivos abertos, é preciso **sair e entrar de novo na sessão** antes de rodar `./iniciar.sh`.

- [ ] **Step 4: Escrever `portas-e-rede.md`**

Título `# Portas e rede`. Específico deste pacote:

- As portas vêm do W-Access; o emulador obedece.
- Não há limite de faixa: a aplicação roda direto no host.
- **Firewall:** o `instalar.sh` já libera a faixa que você informou, no `ufw` ou no `firewalld`. Se você mudar as portas dos controladores no W-Access depois, precisa liberar a faixa nova à mão — mostrar o comando `ufw allow 4000:4499/tcp`.
- **Limite de arquivos abertos:** cada emulador abre um socket; centenas de emuladores estouram o padrão de 1024. O instalador ajusta, mas a sessão precisa ser reaberta.
- **WSL2:** repetir o aviso do modo espelhado, curto, com ponteiro para o capítulo de instalação do WSL2.

- [ ] **Step 5: Preencher o alvo `linux` no manifesto**

```json
      "capitulos": [
        "linux/antes-de-comecar.md",
        "linux/instalar-wsl.md",
        "linux/instalar-emulador.md",
        "comum/conhecer-o-console.md",
        "comum/configurar-wxs.md",
        "comum/validacao.md",
        "linux/portas-e-rede.md",
        "comum/logs.md",
        "comum/problemas.md"
      ]
```

- [ ] **Step 6: Montar, imprimir e conferir os três**

Run: `cd docs/manual && npm run manuais`
Expected: três alvos montados, três PDFs em `.out/`, cada um com pelo menos 10 páginas.

Run: `cd docs/manual && ls -la .out/*.pdf`
Expected: `MANUAL-docker.pdf`, `MANUAL-windows.pdf`, `MANUAL-linux.pdf`.

- [ ] **Step 7: Commit**

```bash
git add docs/manual/
git commit -m "$(cat <<'EOF'
docs(manual): write the Linux and WSL2 package manual

Fecha os três alvos. O capítulo do WSL2 carrega o aviso que mais gera
chamado: por padrão só a própria máquina alcança os emuladores, o
localhost funciona, a tela abre, e o Site Controller de outro computador não
conecta com nada indicando o porquê.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Integração no build dos pacotes

Última tarefa. Depois dela, cada ZIP sai com o seu PDF ao lado do `LEIA-ME.txt`, e a referência quebrada ao `MANUAL.md` inexistente morre.

**Files:**
- Modify: `packaging/build-pacotes.bat`
- Modify: `packaging/docker/LEIA-ME.txt`
- Modify: `packaging/windows/LEIA-ME.txt`
- Modify: `packaging/linux/LEIA-ME.txt`
- Create: `docs/manual/ferramentas/integridade.test.mjs`

**Interfaces:**
- Consumes: `docs/manual/.out/MANUAL-<alvo>.pdf` das Tasks 6, 7 e 8.
- Produces: alvo `manuais` em `build-pacotes.bat`; PDF na raiz de cada ZIP.

- [ ] **Step 1: Escrever o teste de integridade**

`docs/manual/ferramentas/integridade.test.mjs`:

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile, readdir } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const RAIZ = resolve(dirname(fileURLToPath(import.meta.url)), '..')

// Todo capitulo declarado no manifesto tem de existir, e todo capitulo no
// disco tem de estar declarado em ao menos um alvo. Capitulo orfao e
// trabalho escrito que ninguem le.
test('manifesto e disco concordam', async () => {
  const manifesto = JSON.parse(await readFile(join(RAIZ, 'manifesto.json'), 'utf8'))

  const declarados = new Set()
  for (const alvo of Object.values(manifesto.alvos)) {
    for (const c of alvo.capitulos) declarados.add(c)
  }

  const noDisco = new Set()
  for (const pasta of ['comum', 'docker', 'windows', 'linux']) {
    let arquivos = []
    try {
      arquivos = await readdir(join(RAIZ, 'conteudo', pasta))
    } catch {
      continue
    }
    for (const a of arquivos) {
      if (a.endsWith('.md')) noDisco.add(`${pasta}/${a}`)
    }
  }

  for (const d of declarados) {
    assert.ok(noDisco.has(d), `declarado no manifesto e ausente do disco: ${d}`)
  }
  for (const d of noDisco) {
    assert.ok(declarados.has(d), `capitulo orfao, nao declarado em nenhum alvo: ${d}`)
  }
})

test('nenhum capitulo escreve numeracao a mao', async () => {
  for (const pasta of ['comum', 'docker', 'windows', 'linux']) {
    let arquivos = []
    try {
      arquivos = await readdir(join(RAIZ, 'conteudo', pasta))
    } catch {
      continue
    }
    for (const a of arquivos.filter((x) => x.endsWith('.md'))) {
      const texto = await readFile(join(RAIZ, 'conteudo', pasta, a), 'utf8')
      const titulo = texto.split(/\r?\n/).find((l) => l.startsWith('# '))
      assert.ok(titulo, `${pasta}/${a} nao tem titulo de capitulo`)
      assert.doesNotMatch(
        titulo,
        /^#\s+\d+[.)]/,
        `${pasta}/${a} escreve o numero do capitulo a mao — a numeracao e gerada`
      )
    }
  }
})

test('os tres alvos existem no manifesto', async () => {
  const manifesto = JSON.parse(await readFile(join(RAIZ, 'manifesto.json'), 'utf8'))
  for (const alvo of ['docker', 'windows', 'linux']) {
    assert.ok(manifesto.alvos[alvo], `alvo ausente: ${alvo}`)
    assert.ok(manifesto.alvos[alvo].capitulos.length > 0, `alvo vazio: ${alvo}`)
  }
})
```

- [ ] **Step 2: Rodar e confirmar que passa**

Run: `cd docs/manual && npm test`
Expected: PASS. Se acusar capítulo órfão, ou o capítulo entra num alvo, ou sai do disco.

- [ ] **Step 3: Acrescentar o alvo `manuais` ao `build-pacotes.bat`**

Em `packaging/build-pacotes.bat`, no bloco de despacho de alvos logo depois de `if /i "%ALVO%"=="linux"   goto build_linux`:

```bat
if /i "%ALVO%"=="manuais" goto build_manuais
```

E o bloco novo, inserido **antes** de `REM ==================== DOCKER ====================`:

```bat
REM ==================== MANUAIS ====================
:build_manuais
echo.
echo [manuais] Montando e imprimindo os tres PDFs ...
where npm >nul 2>&1
if errorlevel 1 (
    echo [ERRO] npm nao encontrado. Instale o Node.js em https://nodejs.org/
    exit /b 1
)
if not exist docs\manual\node_modules (
    echo [manuais] Instalando dependencias ^(uma vez^) ...
    pushd docs\manual
    call npm install --ignore-scripts
    popd
)
pushd docs\manual
call npm run manuais
set RC=%ERRORLEVEL%
popd
if not "%RC%"=="0" (
    echo [ERRO] Falha ao gerar os manuais.
    exit /b 1
)
echo [manuais] OK: docs\manual\.out\MANUAL-*.pdf
if /i not "%ALVO%"=="todos" goto fim
```

E, no despacho, `todos` passa a começar pelos manuais:

```bat
if /i "%ALVO%"=="todos"   goto build_manuais
```

O batch executa rótulos em sequência, então com `ALVO=todos` a execução cai de `:build_manuais` em `:build_docker` naturalmente, desde que o bloco dos manuais venha imediatamente antes do bloco docker.

- [ ] **Step 4: Copiar o PDF para dentro de cada pacote**

Em cada um dos três blocos de build, junto das linhas que copiam o `LEIA-ME.txt`:

No `:build_docker`:
```bat
if exist docs\manual\.out\MANUAL-docker.pdf copy /Y docs\manual\.out\MANUAL-docker.pdf "%STAGE%\MANUAL.pdf" >nul
```

No `:build_windows`:
```bat
if exist docs\manual\.out\MANUAL-windows.pdf copy /Y docs\manual\.out\MANUAL-windows.pdf "%STAGE%\MANUAL.pdf" >nul
```

No `:build_linux`:
```bat
if exist docs\manual\.out\MANUAL-linux.pdf copy /Y docs\manual\.out\MANUAL-linux.pdf "%STAGE%\MANUAL.pdf" >nul
```

O `if exist` é deliberado: gerar só o pacote, sem ter rodado os manuais antes, continua funcionando — o ZIP sai sem o PDF em vez de o build falhar.

- [ ] **Step 5: Corrigir os três `LEIA-ME.txt`**

Nos três arquivos, a última linha hoje é `O manual completo esta no arquivo MANUAL.md do projeto.` — e esse arquivo nunca existiu. Trocar por:

```
O manual completo, com imagens, esta no MANUAL.pdf ao lado deste arquivo.
```

- [ ] **Step 6: Gerar e conferir**

Run: `packaging\build-pacotes.bat manuais`
Expected: `[manuais] OK: docs\manual\.out\MANUAL-*.pdf`

Run: `packaging\build-pacotes.bat linux`
Expected: `[linux] OK: packaging\.out\GoFacialEmulator-linux.zip`

Run: `powershell -NoProfile -Command "Add-Type -A System.IO.Compression.FileSystem; [IO.Compression.ZipFile]::OpenRead((Resolve-Path packaging\.out\GoFacialEmulator-linux.zip)).Entries | Select-Object -Expand FullName"`
Expected: `MANUAL.pdf` na raiz do ZIP, ao lado de `LEIA-ME.txt`.

Os alvos `docker` e `windows` precisam de Docker rodando e da rede para o download do PostgreSQL portátil; se não estiverem disponíveis, registrar como pendente.

- [ ] **Step 7: Commit**

```bash
git add packaging/ docs/manual/
git commit -m "$(cat <<'EOF'
feat(packaging): ship the PDF manual inside each package

Cada ZIP passa a levar o manual do seu próprio pacote, ao lado do LEIA-ME.

Os três LEIA-ME apontavam para um MANUAL.md que nunca existiu. Agora
apontam para o PDF que está de fato ao lado deles.

O alvo "manuais" roda sozinho, então dá para regerar a documentação sem
reconstruir pacote nenhum. A cópia é condicional: gerar só um pacote, sem
ter rodado os manuais antes, continua funcionando.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

## Cobertura da spec

| Seção 8 da spec | Tarefa |
|---|---|
| 8.1 Estrutura de fontes | Task 1 |
| 8.2 Toolchain | Tasks 1 e 2 |
| 8.3 `montar.mjs` | Task 1 |
| 8.4 `capturar.mjs` | Tasks 3 e 4 |
| 8.5 Conteúdo dos três PDFs | Tasks 5, 6, 7 e 8 |
| 8.6 Integração no build | Task 9 |
| 8.7 Testes | Distribuídos: Task 1 (montagem), Task 2 (PDF), Task 3 (callout), Task 9 (integridade) |

Desvios conscientes da spec, todos registrados aqui:

- **Três dependências, não duas.** `marked` entra porque converter Markdown à mão seria pior que a dependência.
- **`manifesto.json`, não `manifesto.yaml`.** Node lê JSON sem dependência; YAML exigiria uma quarta.
- **Imagens por URL `file://` absoluta** no HTML intermediário, resolvidas por `montar.mjs`. Evita jogo de caminhos relativos entre `.out/` e `ativos/`.
- **Contagem de páginas lida do DOM** (`.pagedjs_page`), não de um parser de PDF, que seria uma quinta dependência.

## Pendências herdadas

A pendência 1 da spec — as seis a dez capturas de aplicativo nativo — é resolvida por construção: elas viram caixa tracejada e linha em `CAPTURAS-PENDENTES.md`, gerado pela Task 1. Sair da pendência exige alguém com a máquina na mão.

As capturas automatizadas das Tasks 3 e 4 exigem ambiente vivo. Se ele não estiver disponível, os scripts são escritos e validados estaticamente, e os PDFs saem com placeholder — nada mais é bloqueado.
