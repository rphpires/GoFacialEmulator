# Console UI Redesign — Design Spec

**Data:** 2026-08-22
**Status:** aprovado (direção e escopo confirmados pelo usuário)
**Base:** `main`

## Problema

A interface web do emulador acumula três defeitos independentes que se
reforçam:

1. **A camada de tempo real está mal fiada.** Existe SSE funcional no
   backend, mas o front abre duas conexões por aba, escreve os contadores
   em IDs que não existem na página, nunca recebe snapshot inicial e trata
   queda de conexão pintando "Offline" por cima dos números reais.
2. **Não há sistema de design.** Bootstrap default com sobreposições
   pontuais. A mesma cor aparece em três sintaxes espalhadas por seis
   arquivos, e o dark mode existe só para o cabeçalho.
3. **Assets vêm de CDN num produto distribuído offline.** Bootstrap,
   Bootstrap Icons, jQuery e Font Awesome são baixados da internet,
   enquanto todo o resto do app é `go:embed`. Sem rede, o app abre sem
   estilo.

Além disso o backend já expõe endpoints que a UI nunca mostrou
(`GET /api/devices/:id/users`, `GET /api/devices/:id/settings`,
`PUT /api/devices/:id/settings`), e a tabela esconde o campo `model`
(hikvision/dahua) que já vem no contexto do template.

## Achados que a implementação deve corrigir

Cada item abaixo é verificável no código atual.

### Tempo real

| # | Defeito | Local |
|---|---------|-------|
| R1 | Duas conexões `EventSource('/events')` por aba | `header.js:66`, `devices.html:315` |
| R2 | `updateCounters()` escreve em `#counter_running`/`#counter_stopped`/`#counter_total`, que só existem em `metrics.html` — template parseado mas nunca renderizado | `devices.html:398` |
| R3 | `#totalText` ("Total: N Emuladores") nunca é atualizado por nada | `header.html:22`, `header.js` |
| R4 | SSE não envia snapshot no connect; só emite em mudança de estado | `handlers.go:884` |
| R5 | Sem heartbeat, sem `retry:`, sem `X-Accel-Buffering: no` | `handlers.go:869-873` |
| R6 | `onerror` pinta "Offline" e apaga as contagens reais (`running_count` chega `undefined`) | `header.js:78` |
| R7 | Polling de 15s roda em paralelo ao SSE saudável — duas fontes de verdade | `header.js:59` |
| R8 | Página de comparação não tem tempo real; o botão Refresh dispara e não dá retorno nem recarrega | `comparison.html:131` |
| R9 | `refresh_counter` definido duas vezes; a segunda definição é um `alert()` morto | `comparison.html:131,165` |
| R10 | Contadores do header agregam 2 estados; a tabela mostra 3 (`disabled` some dentro de `stopped`) | `handlers.go` getSystemStatus |

### Estados e feedback

| # | Defeito | Local |
|---|---------|-------|
| S1 | `alert()` bloqueante para todo resultado de ação | `devices.html`, `sidebar.html` |
| S2 | `App.notify()` (toast) existe e nunca é usado — camada utilitária morta | `main.js:169` |
| S3 | Iniciar/Parar Todos só faz `console.log` no sucesso | `sidebar.html:141,163` |
| S4 | Start/stop de linha é fire-and-forget, sem estado pendente | `devices.html:410,437` |
| S5 | Sem empty state: `{{ range .devices }}` não tem `else` | `devices.html:104` |
| S6 | `c.HTML(500, "error.html")` em 4 lugares, mas `error.html` não existe e o `HTMLRender` do gin nunca foi configurado → panic → 500 branco | `handlers.go:309,317,470,1006` |
| S7 | Botões "desabilitados" são `<a href="#">` com classe cosmética: continuam focáveis e clicáveis | `devices.html:118-131` |
| S8 | Estado do menu lateral e dos filtros não persiste entre navegações | `sidebar.html:113` |
| S9 | `page_range` é iterado nos templates mas nenhum handler o define → números de página nunca renderizam | `devices.html:172`, `comparison.html:89` |

### Visual e layout

| # | Defeito | Local |
|---|---------|-------|
| V1 | Sem tokens: `#ff8c00` no CSS, `#117529`/`#a70000` inline nos templates, `#10b981`/`#f59e0b`/`#ef4444` no JS | 6 arquivos |
| V2 | Status é texto com borda e `style` inline, repetido em 3 templates e duplicado em template string no JS | `devices.html:137-153,368` |
| V3 | `.status-running`/`.status-stopped`/`.status-disabled` definidos e nunca usados | `main.css:113-128` |
| V4 | Dark mode só do cabeçalho: barra escura sobre app branco | `header.css:317` |
| V5 | Menu lateral animado por escrita direta de `style.width`/`justify-content`/`display` em JS | `sidebar.html:63-113` |
| V6 | `main` (card com sombra) contendo `.card` (outra sombra) | `main.css:31,45` |
| V7 | 9 colunas todas `text-center`, sem `tabular-nums`, sem header fixo | `devices.html:87-97` |
| V8 | `<576px` esconde o texto dos badges, deixando pontos sem rótulo acessível | `header.css:281` |
| V9 | Tabela de 9 colunas sem container com rolagem horizontal no mobile | `devices.html:86` |
| V10 | Hierarquia inconsistente: `h4`, `h3.display-8` (classe inexistente no BS5), `h2` | 3 templates |
| V11 | `bg-primary text-black` — azul Bootstrap, texto preto, briga com a marca | `settings.html:19,88` |
| V12 | Footer com ano fixo "© 2024" | `footer.html:3` |
| V13 | `model` (hikvision/dahua) e `ip_address` chegam no contexto e não são exibidos | `handlers.go:718-726` |

### Entrega e arquitetura

| # | Defeito | Local |
|---|---------|-------|
| D1 | Bootstrap CSS+JS, Bootstrap Icons, jQuery e Font Awesome vindos de CDN num produto offline | `base.html:10-13`, `metrics.html:1` |
| D2 | jQuery (~90KB) carregado para um `$.ajaxSetup` e um plugin em template morto | `main.js:130` |
| D3 | Dois sistemas de ícone (Font Awesome 4.0.3 + Bootstrap Icons) | `metrics.html`, `base.html` |
| D4 | `loadTemplate()` re-parseia 6 arquivos do embed a cada request | `handlers.go:186-196` |
| D5 | `metrics.html` e `pagination.html` parseados em toda página, nunca invocados | `handlers.go:191-192` |
| D6 | CSS e JS embutidos em `<style>`/`<script>` por template: sem cache, regras duplicadas, briga de especificidade (`.collapse` sobrescreve o do Bootstrap) | `sidebar.html`, `devices.html`, `comparison.html` |
| D7 | Senha do W-Access renderizada no atributo `value=` do HTML | `settings.html:49` |
| D8 | `result.error` do servidor injetado via `innerHTML` | `settings.html:155,158` |

## Direção de design

**Console de bancada, não dashboard SaaS.** O público é engenheiro de campo
e QA rodando dezenas de emuladores ao lado de um terminal e de um tail de
log. A interface deve ler como instrumento: densa, escura, com estado
legível de relance e ação sempre a um clique.

Compromisso deliberado com **tema escuro único**. Não é preferência
estética: o dark mode atual está meio implementado e a escolha resolve
V4 de vez em vez de dobrar a superfície de tema. Toda cor é pintada
explicitamente — nada herda o fundo do agente do usuário.

### Tokens de cor

```css
--ink-900: #0e1116;  /* fundo da aplicação */
--ink-800: #151a22;  /* superfície de painel */
--ink-700: #1c222c;  /* superfície elevada: topbar, cabeçalho de tabela */
--ink-600: #262d3a;  /* borda padrão */
--ink-500: #333c4d;  /* borda em hover / divisor forte */

--text-hi:  #e6ebf2;  /* texto primário */
--text-mid: #9aa6ba;  /* rótulos, texto secundário */
--text-low: #6b7688;  /* texto desabilitado, marca d'água */

--signal:    #ff8c00;  /* laranja Invenzi — SOMENTE ação e foco */
--signal-hi: #ffa733;  /* hover de ação */
--signal-dim:#40301a;  /* fundo de ação em repouso */

--live: #3ddc97;  /* running */
--halt: #ff5c5c;  /* stopped */
--idle: #58637a;  /* disabled */
--warn: #ffc247;  /* divergência, erro de comparação */
```

Regra de uso, e ela é o ponto: **laranja nunca comunica estado.** Estado é
verde/vermelho/cinza. Laranja é exclusivo de "você pode agir aqui" e do anel
de foco. Isso elimina a ambiguidade atual, onde laranja é ao mesmo tempo
marca, botão primário e destaque.

### Tipografia

- **IBM Plex Sans** 400/500/600 — interface. Desenhada para software de
  operação, não é a família default de dashboard.
- **IBM Plex Mono** 400/500 — IDs, portas, contagens, intervalos,
  timestamps. Sempre com `font-variant-numeric: tabular-nums`, para as
  colunas numéricas alinharem verticalmente.

Ambas embedadas como `woff2` em `assets/web/static/fonts/`, servidas pelo
`go:embed` existente. Zero requisição externa em runtime.

Escala: 12 / 13 / 15 / 20 / 28px. Rótulos de tabela em 12px, 500,
`letter-spacing: .04em`, maiúsculas.

### Layout

```
┌─ Facial Emulators v1.4 ──── ▇▇▇▇▇▇▁▁▁▁░░ 12/40 ──── ⚙ ── INVENZI ─┐
│▌                                                                  │
│▌ Dispositivos                          [ ▶ Iniciar ] [ ■ Parar ]  │
│▌                                                                  │
│▌ id ____  nome __________  porta ____  [Filtrar] [Limpar]         │
│▌                                                                  │
│▌ ☐  ID    NOME             MODELO   PORTA  LOG  INT  USERS  ESTADO│
│▌ ☐ ●123   Portaria Norte   hikvision 7070   ✓   30    412   vivo  │
│▌ ☐ ○124   Doca 2           dahua     7071   ✓   30    118   off   │
│▌ ☐ ◌125   Recepção         hikvision 7072   —    —      0   desab.│
│▌                                                                  │
│▌ 10 por página                     ‹ 1 2 3 ›                      │
└───────────────────────────────────────────────────────────────────┘
```

- **Rail esquerdo** fixo em 64px, expansível a 216px, estado persistido em
  `localStorage`, alternado por classe CSS (não por escrita de estilo em
  JS). Item ativo marcado por barra laranja de 2px na borda esquerda.
- **Topbar** com o *fleet meter* no lugar dos dois badges.
- **Filtros inline**, sempre visíveis, uma linha só. O card colapsável
  atual custa dois cliques e um estado a mais para três campos de texto.
- **Tabela densa**: linhas de 40px, cabeçalho fixo ao rolar, `model`
  exposto, números alinhados à direita em mono tabular, nome alinhado à
  esquerda. Envolvida em container com `overflow-x: auto`.

### Elemento assinatura: fleet meter + LED de linha

Uma barra segmentada na topbar, um segmento por emulador, colorido por
estado. Quarenta emuladores viram quarenta traços — a frota inteira legível
num relance, sem ler número nenhum.

O mesmo vocabulário desce para a tabela: cada linha tem um LED do mesmo
estado à esquerda do ID. Quando o SSE entrega uma mudança, o LED da linha
pisca uma vez e o segmento correspondente do meter transiciona de cor.

Isso resolve a queixa de fundo, e não só o bug: **o tempo real vira visível.**
Hoje, mesmo que o SSE funcionasse, a mudança apareceria como um texto que
troca sem aviso. Toda a ousadia do design fica gasta aqui; o resto — tabela,
filtros, formulários — permanece contido e sem ornamento.

Respeita `prefers-reduced-motion`: com redução ativa, o LED troca de cor sem
o flash e o meter transiciona sem animação.

## Arquitetura alvo

### Assets

```
assets/web/static/
  css/
    tokens.css       cores, tipografia, espaçamento, raio, sombra
    base.css         reset, elementos, tipografia base, utilitários
    layout.css       shell: topbar, rail, região de conteúdo
    components.css   botão, campo, tabela, badge, drawer, toast, meter
  js/
    realtime.js      FleetStream — conexão SSE única + pub/sub
    toast.js         notificações não bloqueantes
    app.js           bootstrap do shell: rail, tema, ações globais
    devices.js       tabela de dispositivos, seleção, ações, drawer
    comparison.js    tabela de comparação e refresh
    settings.js      formulário de configurações
  fonts/             IBMPlexSans-{400,500,600}.woff2, IBMPlexMono-{400,500}.woff2
  icons.svg          sprite SVG (~15 símbolos)
```

Zero `<style>` e zero `<script>` inline nos templates. Bootstrap, Bootstrap
Icons, jQuery e Font Awesome saem por completo.

### Tempo real

Uma conexão por aba, exposta como singleton:

```js
FleetStream.subscribe('snapshot', fn)  // estado completo da frota
FleetStream.subscribe('device', fn)    // mudança de um dispositivo
FleetStream.subscribe('status', fn)    // 'live' | 'reconnecting' | 'down'
```

Backend em `/events`:

- `retry: 3000` como primeira linha do stream.
- `event: snapshot` imediatamente no connect, com a frota inteira.
- `event: device` a cada mudança de estado, com o dispositivo e as
  contagens agregadas dos três estados.
- Comentário `: keepalive` a cada 20s, para a conexão não morrer ociosa.
- Header `X-Accel-Buffering: no`.

O cliente reconecta com backoff (1s, 2s, 4s, 8s, teto de 30s). Enquanto
estiver reconectando, a UI marca os dados como possivelmente defasados, em
vez de sobrescrevê-los com "Offline". Só depois de 30s sem stream é que
entra um polling de `/api/status` como rede de segurança — e ele para
assim que o stream volta.

### Contagens

`GET /api/status` e o payload do SSE passam a expor os três estados
separados: `running`, `stopped`, `disabled`, mais `total`. Isso alinha o
header com a tabela (R10) e alimenta o meter.

### Templates

- Parseados uma vez no startup, guardados em `map[string]*template.Template`
  no `Handler` (D4).
- `metrics.html` e `pagination.html` removidos (D5).
- `error.html` criado, com o mesmo shell, e usado via o cache de templates —
  não via `c.HTML`, que exige um `HTMLRender` que este app não configura (S6).
- `page_range` passa a ser calculado no backend e incluído no contexto de
  `mainPage` e `comparisonPage` (S9).

### Detalhe do dispositivo

Um drawer lateral abre ao clicar na linha. Duas abas:

- **Usuários** — `GET /api/devices/:id/users`, com busca e paginação, que
  o endpoint já suporta.
- **Configurações** — `GET /api/devices/:id/settings` para leitura e
  `PUT /api/devices/:id/settings` para gravar. É por aqui que o intervalo
  de eventos deixa de ser read-only.

## Fora de escopo

- Tema claro. A decisão é dark único; os tokens ficam em um único bloco
  `:root`, o que torna um tema futuro uma troca de valores, não uma
  refatoração.
- Traduzir a interface. Continua em pt-BR.
- Alterar o protocolo dos emuladores ou qualquer lógica de dispositivo.
- Mexer nas páginas de `/monitoring/*`, que servem JSON.

## Critérios de aceite

1. Nenhuma requisição a host externo em runtime: `grep -r "https://" assets/web/templates/` não retorna nada.
2. Uma única conexão a `/events` por aba, verificável no painel de rede.
3. Abrir a página com emuladores rodando mostra as contagens corretas sem
   nenhuma interação — o snapshot chega no connect.
4. Iniciar um emulador em outra aba atualiza LED, meter e contagens desta
   aba em menos de um segundo, sem recarregar.
5. Derrubar o servidor marca o stream como reconectando sem zerar os
   números; subir de volta restaura o estado sozinho.
6. Filtro sem resultado mostra empty state com ação para limpar o filtro.
7. Falha de banco na página inicial renderiza `error.html`, não 500 branco.
8. Botão de dispositivo desabilitado não é focável nem acionável por teclado.
9. Números de página renderizam.
10. `go build ./...` e `go test ./...` passam.
