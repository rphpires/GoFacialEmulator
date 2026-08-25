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

// desenharNumeros desenha o numero de cada marcador como <div> real no
// documento, por cima de tudo.
//
// O ::after de cssDeCallout() pinta bem na maioria dos elementos, mas o
// Chromium simplesmente nao renderiza conteudo gerado (::before/::after) em
// elementos substituidos — <select>, <input>, <textarea> — mesmo com o
// computed style correto (content, position, z-index todos certos: foi
// verificado num script isolado antes de concluir que era isso). Como boa
// parte da tela e feita de <input> e <select> (filtros, campos de
// configuracao, o proprio seletor de Modo), o numero ficava invisivel nessas
// figuras, so o outline aparecia.
//
// O numero tambem some quando o marcador fica colado na borda do elemento
// ou da pagina: o circulo nasce 14px para fora da caixa do alvo, e tanto
// um recorte cravado na caixa do elemento (elementHandle.screenshot) quanto
// a borda esquerda/superior da propria pagina cortam esse tanto.
//
// A correcao e desenhar o numero como elemento de verdade, posicionado via
// getBoundingClientRect e sempre dentro da viewport — nao depende de
// ::after pintar, e nao ha caixa de elemento para cortar. Quando o ::after
// tambem pinta (caso comum, no botao/div/tabela), os dois ficam exatamente
// sobrepostos e o resultado visual e identico a um so numero.
async function desenharNumeros(page, marcadores) {
  await page.evaluate((marcadores) => {
    const RAIO = 14
    const DIAMETRO = RAIO * 2
    document.querySelectorAll('[data-callout-numero]').forEach((n) => n.remove())

    // Dois alvos vizinhos (o titulo da gaveta e o LED dentro dele, por
    // exemplo) podem ter cantos superior-esquerdos a poucos pixels um do
    // outro — os dois circulos nasceriam praticamente no mesmo lugar, e o
    // desenhado por ultimo cobre o de baixo por completo. Por isso cada
    // badge novo se afasta dos ja colocados antes de ser fixado na tela.
    const colocados = []
    function afastarColisao (x, y) {
      let tentativas = 0
      while (
        colocados.some((p) => Math.abs(p.x - x) < DIAMETRO && Math.abs(p.y - y) < DIAMETRO) &&
        tentativas < 20
      ) {
        x += DIAMETRO * 0.6
        tentativas += 1
      }
      colocados.push({ x, y })
      return { x, y }
    }

    for (const { seletor, numero } of marcadores) {
      document.querySelectorAll(seletor).forEach((alvo) => {
        const r = alvo.getBoundingClientRect()
        const bruto = {
          x: Math.max(0, r.left - RAIO),
          y: Math.max(0, r.top - RAIO)
        }
        const pos = afastarColisao(bruto.x, bruto.y)
        const badge = document.createElement('div')
        badge.setAttribute('data-callout-numero', '')
        badge.textContent = String(numero)
        Object.assign(badge.style, {
          position: 'fixed',
          left: pos.x + 'px',
          top: pos.y + 'px',
          width: DIAMETRO + 'px',
          height: DIAMETRO + 'px',
          borderRadius: '50%',
          background: '#d00',
          color: '#fff',
          font: 'bold 16px/28px system-ui, sans-serif',
          textAlign: 'center',
          zIndex: 2147483647,
          pointerEvents: 'none'
        })
        document.body.appendChild(badge)
      })
    }
  }, marcadores)
}

// aplicarCallouts troca os callouts da captura anterior pelos desta, em vez
// de empilhar. page.addStyleTag() sempre cria uma tag <style> nova; como as
// capturas 8 e 9 reaproveitam a mesma pagina da 7 (url: null, gaveta
// continua aberta), tags acumuladas deixavam o outline e o numero de uma
// captura anterior colados na proxima. Um <style> fixo, com o conteudo
// sobrescrito a cada chamada, resolve — inclusive limpando tudo quando a
// proxima captura nao tem marcador nenhum.
async function aplicarCallouts(page, marcadores) {
  const css = marcadores.length > 0 ? cssDeCallout(marcadores) : ''
  await page.evaluate((css) => {
    let estilo = document.getElementById('callout-estilo')
    if (!estilo) {
      estilo = document.createElement('style')
      estilo.id = 'callout-estilo'
      document.head.appendChild(estilo)
    }
    estilo.textContent = css
  }, css)
  await desenharNumeros(page, marcadores)
}

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

  await aplicarCallouts(page, marcadores)

  const caminho = join(DESTINO, arquivo)
  if (alvo) {
    // Recorte de um componente so. Uma pagina inteira de 1440x900 reduzida
    // para caber na coluna do PDF deixa o rail ilegivel.
    //
    // page.screenshot({ clip }) em vez de elemento.screenshot(): o segundo
    // crava o recorte exatamente na caixa do elemento e corta fora qualquer
    // numero que escape dela (o circulo nasce 14px para fora). A folga
    // garante que o numero cabe no recorte.
    const elemento = await page.$(alvo)
    if (!elemento) throw new Error(`alvo de recorte nao encontrado: ${alvo}`)
    const caixa = await elemento.boundingBox()
    if (!caixa) throw new Error(`alvo de recorte sem geometria visivel: ${alvo}`)
    const FOLGA = 20
    const clip = {
      x: Math.max(0, caixa.x - FOLGA),
      y: Math.max(0, caixa.y - FOLGA),
      width: caixa.width + FOLGA * 2,
      height: caixa.height + FOLGA * 2
    }
    await page.screenshot({ path: caminho, clip })
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
  // #meter-health so ganha texto quando o FleetStream degrada — a mesma
  // logica do aviso de alcancabilidade (silencio e o estado normal, ver
  // components.css ".meter__health:empty { display: none }"). Com o stream
  // saudavel o elemento existe no DOM mas tem caixa 0x0, e um callout
  // apontado pra ele so cairia fora do recorte. Por isso o terceiro
  // marcador so entra quando ha algo pra apontar.
  await page.goto(`${EMULADOR}/`, { waitUntil: 'networkidle' })
  const saudeVisivel = await page.isVisible('#meter-health')
  const marcadoresMedidor = [
    { seletor: '#meter-bar', numero: 1 },
    { seletor: '#meter-reading', numero: 2 }
  ]
  if (saudeVisivel) {
    marcadoresMedidor.push({ seletor: '#meter-health', numero: 3 })
  } else {
    console.log(
      '[captura] #meter-health vazio neste ambiente (stream saudavel) — ' +
      'emulador-medidor.png sai só com os marcadores 1 e 2'
    )
  }
  await disparar(page, {
    url: null,
    arquivo: 'emulador-medidor.png',
    alvo: '#fleet-meter',
    marcadores: marcadoresMedidor
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
  // da primeira linha. A gaveta usa o atributo `hidden` para visibilidade
  // (device-drawer.js: drawer.hidden = false) e `data-open` so para disparar
  // a transicao de deslizamento em CSS (transform 0.2s) — por isso a espera
  // pelo seletor precisa vir acompanhada de uma pausa curta para a animacao
  // acabar antes da foto, senao a gaveta sai capturada a meio caminho.
  await disparar(page, {
    url: `${EMULADOR}/`,
    arquivo: 'emulador-gaveta-usuarios.png',
    antes: async (p) => {
      await p.click('.device-details-btn')
      await p.waitForSelector('#device-drawer:not([hidden])')
      await p.waitForSelector('#device-drawer[data-open="true"]')
      await p.waitForTimeout(300)
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
