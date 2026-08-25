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

// O polyfill do paged.js vive dentro do pacote, mas o package.json do
// pagedjs so declara condicoes para "." no campo "exports" — pedir o
// subcaminho "pagedjs/dist/paged.polyfill.js" direto e barrado pelo Node
// com ERR_PACKAGE_PATH_NOT_EXPORTED. Por isso resolvemos a entrada
// principal do pacote (permitida pelas condicoes) e subimos ate a raiz do
// pacote, onde dist/, lib/ e src/ sao irmaos.
const RAIZ_PAGEDJS = dirname(dirname(require.resolve('pagedjs')))
const PAGEDJS = join(RAIZ_PAGEDJS, 'dist', 'paged.polyfill.js')

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
