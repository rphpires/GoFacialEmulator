// Monta o HTML de um alvo a partir dos capitulos declarados no manifesto.
//
// O markdown continua sendo a fonte do TEXTO; este arquivo e a camada de
// LAYOUT — capa, sumario paginado, abertura de capitulo, avisos, passos e
// legendas de figura. Quem quiser mexer na aparencia mexe aqui e em
// ativos/css/manual.css, nunca no conteudo.
//
// Regras que valem a pena saber antes de mexer:
//   - capitulo declarado e ausente do disco FALHA o build. Sumico de
//     capitulo e erro, nao divida.
//   - figura ausente NAO falha: vira caixa tracejada e entra na lista de
//     pendencias. Se falhasse, o primeiro PDF nunca seria gerado, porque
//     algumas telas so um humano consegue tirar.
//   - callout com marcador desconhecido FALHA. E defeito de autoria, do
//     mesmo tipo que capitulo sem titulo, e silenciar vira aviso invisivel.
import { readFile, writeFile, mkdir, access } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { marked } from 'marked'

const AQUI = dirname(fileURLToPath(import.meta.url))
const RAIZ_PADRAO = resolve(AQUI, '..')

// As mesmas IBM Plex que o console embarca no binario. Sao lidas do disco
// e apontadas por file:// — o manual e impresso offline, sem CDN, pela
// mesma razao que o console nao tem nenhum.
const PASTA_FONTES = resolve(RAIZ_PADRAO, '..', '..', 'assets', 'web', 'static', 'fonts')

const AVISOS = {
  atencao: { classe: 'aviso-atencao', rotulo: 'Atenção' },
  nota: { classe: 'aviso-nota', rotulo: 'Nota' },
  dica: { classe: 'aviso-dica', rotulo: 'Dica' }
}

// Rotulos do roteiro de validacao. A cor de cada um sai da regra de cor do
// console: laranja e ACAO (o que voce faz), verde e o estado esperado,
// vermelho e o desvio.
const PASSOS = {
  'Faça': 'passo-faca',
  'Tem que acontecer': 'passo-esperado',
  'Se não acontecer': 'passo-desvio'
}

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

function semTags(s) {
  return s.replace(/<[^>]*>/g, '')
}

// caminhoDaFigura resolve o href JA PARSEADO pelo marked (sem o titulo entre
// aspas, que o proprio marked separa) para o caminho absoluto do arquivo em
// ativos/. Ponto unico de conversao href -> caminho: nao duplicar esta conta
// em outro lugar, ou os dois lados podem voltar a discordar sobre o que
// existe no disco.
function caminhoDaFigura(raiz, href) {
  return join(raiz, 'ativos', href.replace(/^ativos\//, ''))
}

function fontesCss() {
  const arquivos = {
    sans: join(PASTA_FONTES, 'IBMPlexSans-Variable.woff2'),
    monoRegular: join(PASTA_FONTES, 'IBMPlexMono-Regular.woff2'),
    monoMedium: join(PASTA_FONTES, 'IBMPlexMono-Medium.woff2')
  }
  // Sem as fontes no disco o manual sai com a fonte do sistema em vez de
  // falhar: o texto continua legivel, e o build roda em maquina que so tem
  // a pasta docs/manual (os testes, por exemplo).
  if (!Object.values(arquivos).every((a) => existsSync(a))) return ''

  const url = (a) => pathToFileURL(a).href
  return `
@font-face {
  font-family: 'IBM Plex Sans';
  src: url('${url(arquivos.sans)}') format('woff2-variations');
  font-weight: 400 600;
  font-style: normal;
}
@font-face {
  font-family: 'IBM Plex Mono';
  src: url('${url(arquivos.monoRegular)}') format('woff2');
  font-weight: 400;
  font-style: normal;
}
@font-face {
  font-family: 'IBM Plex Mono';
  src: url('${url(arquivos.monoMedium)}') format('woff2');
  font-weight: 500;
  font-style: normal;
}
`
}

// barrasDaCapa desenha o medidor de frota da topbar do console como marca
// grafica da capa. As alturas sao fixas de proposito: capa que muda de
// desenho a cada build nao e capa, e um ruido no diff.
function barrasDaCapa() {
  const destaque = new Set([2, 3, 9, 14, 15, 22, 30, 31, 37])
  const alturas = [14, 20, 26, 34, 42, 50, 58]
  const barras = []
  for (let i = 0; i < 40; i += 1) {
    const altura = alturas[(i * 7 + 3) % alturas.length]
    const cor = destaque.has(i) ? '#ff8c00' : altura > 34 ? '#c8cfd9' : '#dfe4ea'
    barras.push(
      `<div class="capa-barra" style="height: ${altura / 4}mm; background: ${cor}"></div>`
    )
  }
  return barras.join('\n      ')
}

function mesEAno(data = new Date()) {
  const texto = data.toLocaleDateString('pt-BR', { month: 'long', year: 'numeric' })
  return texto.charAt(0).toUpperCase() + texto.slice(1)
}

// renderizarCapitulo converte um capitulo em HTML e resolve as figuras.
// Devolve tambem as subsecoes (para o sumario e para a abertura) e as
// pendencias encontradas, para o checklist.
async function renderizarCapitulo(raiz, caminhoRelativo, numeroCapitulo) {
  const caminhoAbsoluto = join(raiz, 'conteudo', caminhoRelativo)
  if (!(await existe(caminhoAbsoluto))) {
    throw new Error(
      `capitulo declarado no manifesto mas ausente do disco: ${caminhoRelativo}`
    )
  }

  const markdown = await readFile(caminhoAbsoluto, 'utf8')
  const pendencias = []
  const subsecoes = []
  let numeroFigura = 0
  let titulo = null

  const renderer = new marked.Renderer()

  // O h1 e engolido aqui: ele volta depois, dentro da faixa de abertura
  // montada por montarAbertura(), que precisa conhecer TODAS as subsecoes
  // do capitulo — e so sabe disso quando a renderizacao termina.
  renderer.heading = (texto, nivel) => {
    if (nivel === 1) {
      titulo = texto
      return ''
    }
    if (nivel === 2) {
      const numerada = semTags(texto).match(/^(\d+)\.\s+(.*)$/)
      const id = `cap-${numeroCapitulo}-s${subsecoes.length + 1}`
      subsecoes.push({
        id,
        titulo: numerada ? numerada[2] : semTags(texto),
        numero: numerada ? numerada[1] : null
      })
      if (numerada) {
        return (
          `<h2 class="secao secao-passo" id="${id}">` +
          `<span class="secao-numero">${numerada[1]}</span>` +
          `<span>${numerada[2]}</span></h2>\n`
        )
      }
      return `<h2 class="secao" id="${id}">${texto}</h2>\n`
    }
    return `<h${nivel}>${texto}</h${nivel}>\n`
  }

  // renderer.image e chamado pelo marked de forma sincrona, com o href JA
  // separado do titulo entre aspas (`![alt](caminho "titulo")`) — por isso
  // a checagem de existencia acontece aqui, com a mesma string que o marked
  // usa, em vez de reanalisar o markdown cru com uma regex propria.
  renderer.image = (href, _tituloImg, alt) => {
    numeroFigura += 1
    const rotulo = `Figura ${numeroCapitulo}.${numeroFigura}`
    const caminhoImagem = caminhoDaFigura(raiz, href)
    const legenda =
      `<figcaption><span class="figura-num">${rotulo}</span>` +
      `<span>${escaparHtml(alt)}</span></figcaption>`

    if (!existsSync(caminhoImagem)) {
      pendencias.push({ capitulo: caminhoRelativo, alt, caminho: caminhoImagem })
      return (
        `<figure class="figura figura-pendente">` +
        `<div class="caixa-pendente">` +
        `<span class="pendente-rotulo">Captura pendente</span>` +
        `<span class="pendente-alt">${escaparHtml(alt)}</span></div>` +
        `${legenda}</figure>\n`
      )
    }

    const url = pathToFileURL(caminhoImagem).href
    return (
      `<figure class="figura"><img src="${url}" alt="${escaparHtml(alt)}">` +
      `${legenda}</figure>\n`
    )
  }

  // Um paragrafo que abre com "**Faça:**" (e os dois irmaos dele) nao e um
  // paragrafo: e uma linha de um passo do roteiro. Vira uma faixa rotulada,
  // com o rotulo na cor do papel que ele cumpre.
  renderer.paragraph = (texto) => {
    const passo = texto.match(/^<strong>(Faça|Tem que acontecer|Se não acontecer):<\/strong>\s*/)
    if (passo) {
      const classe = PASSOS[passo[1]]
      return (
        `<div class="passo-linha ${classe}">` +
        `<div class="passo-rotulo">${passo[1]}</div>` +
        `<div class="passo-corpo">${texto.slice(passo[0].length)}</div></div>\n`
      )
    }
    // Figura sozinha no paragrafo sai SEM o <p>: o navegador fecha o
    // paragrafo antes do <figure> de qualquer jeito, e o <p> vazio que
    // sobra separa a figura do passo a que ela pertence.
    if (texto.startsWith('<figure')) return `${texto}\n`
    return `<p>${texto}</p>\n`
  }

  // Callout: `> [!atencao]` / `[!nota]` / `[!dica]` na primeira linha da
  // citacao. Marcador ausente ou desconhecido para o build — aviso que nao
  // aparece como aviso e pior que aviso nenhum.
  renderer.blockquote = (interno) => {
    const marca = interno.match(/^\s*<p>\[!\s*([a-zA-ZçÇãÃáÁéÉíÍ]+)\s*\]\s*/)
    if (!marca) {
      throw new Error(
        `citacao sem marcador de aviso em ${caminhoRelativo} — ` +
        'use "> [!atencao]", "> [!nota]" ou "> [!dica]" na primeira linha'
      )
    }
    const chave = marca[1]
      .toLowerCase()
      .normalize('NFD')
      .replace(/[\u0300-\u036f]/g, '')
    const aviso = AVISOS[chave]
    if (!aviso) {
      throw new Error(
        `marcador de aviso desconhecido em ${caminhoRelativo}: [!${marca[1]}] — ` +
        `conhecidos: ${Object.keys(AVISOS).join(', ')}`
      )
    }
    return (
      `<aside class="aviso ${aviso.classe}">` +
      `<p class="aviso-rotulo">${aviso.rotulo}</p>` +
      `${interno.slice(marca[0].length).replace(/^/, '<p>')}</aside>\n`
    )
  }

  // Bloco de codigo com barra de identificacao. A info string aceita duas
  // partes: "```yaml sistema\docker-compose.yml" mostra o arquivo na
  // esquerda e a linguagem na direita. So a linguagem, mostra a linguagem.
  renderer.code = (codigo, info) => {
    const corpo = `<pre><code>${escaparHtml(codigo)}</code></pre>`
    if (!info) return `<div class="bloco">${corpo}</div>\n`

    const [lang, ...resto] = info.trim().split(/\s+/)
    const rotulo = resto.length > 0 ? resto.join(' ') : lang
    const direita =
      resto.length > 0
        ? `<span class="bloco-barra-acao">${escaparHtml(lang)}</span>`
        : ''
    return (
      `<div class="bloco"><div class="bloco-barra">` +
      `<span>${escaparHtml(rotulo)}</span>${direita}</div>${corpo}</div>\n`
    )
  }

  const html = marked.parse(markdown, { renderer })

  // Sem um cabecalho de nivel 1, nao ha titulo legivel para o sumario nem
  // para a aba do navegador — e todo texto visivel ao usuario final precisa
  // ser portugues sem jargao, nunca um caminho de arquivo cru. Um capitulo
  // sem "# Titulo" e um defeito de autoria, do mesmo tipo que um capitulo
  // ausente do disco: falha o build em vez de vazar o caminho.
  if (titulo === null) {
    throw new Error(
      `capitulo sem cabecalho de titulo ("# ..."): ${caminhoRelativo}`
    )
  }

  return { html, titulo, subsecoes, pendencias }
}

function montarAbertura(numero, titulo, subsecoes) {
  const doisDigitos = String(numero).padStart(2, '0')

  const neste =
    subsecoes.length >= 2
      ? `
    <div class="neste-capitulo">
      <p class="neste-capitulo-rotulo">Neste capítulo</p>
      <ul>
${subsecoes
  .map(
    (s) =>
      `        <li>${s.numero ? `${s.numero}. ` : ''}${escaparHtml(s.titulo)}</li>`
  )
  .join('\n')}
      </ul>
    </div>`
      : ''

  return `<header class="abertura">
    <div class="abertura-topo">
      <div class="abertura-numero">${doisDigitos}</div>
      <div>
        <p class="abertura-rotulo">Capítulo</p>
        <h1>${titulo}</h1>
      </div>
    </div>${neste}
  </header>`
}

function montarSumario(config, capitulos) {
  const entradas = capitulos
    .map((c) => {
      const subs =
        c.subsecoes.length > 0
          ? `
      <ul class="sumario-subs">
${c.subsecoes
  .map(
    (s) =>
      `        <li><a href="#${s.id}">` +
      `<span class="sumario-titulo">${s.numero ? `${s.numero}. ` : ''}${escaparHtml(s.titulo)}</span>` +
      `<span class="sumario-guia"></span></a></li>`
  )
  .join('\n')}
      </ul>`
          : ''
      return `    <li class="sumario-cap">
      <a href="#cap-${c.numero}">
        <span class="sumario-num">${c.numero}</span>
        <span class="sumario-titulo">${escaparHtml(semTags(c.titulo))}</span>
        <span class="sumario-guia"></span>
      </a>${subs}
    </li>`
    })
    .join('\n')

  return `<nav id="sumario">
  <h2>Sumário</h2>
  <div class="sumario-regua"></div>
  <ol class="sumario-lista">
${entradas}
  </ol>
</nav>`
}

function montarCapa(config, outrosPacotes) {
  return `<section class="capa">
  <div class="capa-topo">
    <span class="capa-produto">GoFacial Emulator</span>
    <span class="capa-selo">Documento incluído no pacote</span>
  </div>

  <div class="capa-meio">
    <div class="capa-barras">
      ${barrasDaCapa()}
    </div>
    <div>
      <div class="capa-linha-1">Manual de instalação</div>
      <div class="capa-linha-2">${escaparHtml(config.pacote ?? config.titulo)}</div>
      <div class="capa-regua"></div>
      <p class="capa-resumo">${escaparHtml(config.resumo ?? config.subtitulo)}</p>
    </div>
  </div>

  <div class="capa-rodape">
    <div class="capa-rodape-regua"></div>
    <div class="capa-meta">
      <div><span>Publicado em</span><b>${mesEAno()}</b></div>
      <div><span>Formato</span><b>A4 · impressão</b></div>
      <div><span>Também disponível para</span><b>${escaparHtml(outrosPacotes || '—')}</b></div>
    </div>
  </div>
</section>`
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
    capitulos.push({
      numero: n,
      titulo: cap.titulo,
      subsecoes: cap.subsecoes,
      html: cap.html
    })
    pendencias.push(...cap.pendencias)
  }

  const outrosPacotes = Object.entries(manifesto.alvos)
    .filter(([nome]) => nome !== alvo)
    .map(([nome, c]) => (c.pacote ?? nome).replace(/^Pacote\s+/i, ''))
    .join(' · ')

  const cabecalho = config.cabecalho ?? config.titulo

  const corpo = capitulos
    .map(
      (c) =>
        `<section class="capitulo" id="cap-${c.numero}">\n` +
        `  <span class="marcador-corrente">${escaparHtml(
          `${c.numero} · ${semTags(c.titulo)}`
        )}</span>\n` +
        `  ${montarAbertura(c.numero, c.titulo, c.subsecoes)}\n${c.html}</section>`
    )
    .join('\n')

  const html = `<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<title>${escaparHtml(config.titulo)}</title>
<style>${fontesCss()}</style>
<style>${css}</style>
<style>@page { @top-right { content: "${escaparHtml(cabecalho).replace(/"/g, '')}"; } }</style>
</head>
<body>
${montarCapa(config, outrosPacotes)}
${montarSumario(config, capitulos)}
<main id="conteudo">
${corpo}
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
