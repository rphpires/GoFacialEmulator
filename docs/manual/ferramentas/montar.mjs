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
