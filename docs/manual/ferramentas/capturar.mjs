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

// Tempo maximo de espera pelo goto() inicial e pelo POST de login do
// W-Access antes de desistir e avisar, em vez de travar a captura inteira.
// Generoso, mas finito: um round-trip saudavel de WebForms na mesma rede
// (IIS local, sem internet no meio) fica na casa de 1-3s, entao 20s ja da
// folga larga pra variacao de carga. Finito porque a investigacao registrada
// em task-4-report.md confirmou que, com o servico de backend fora do ar, o
// POST de login nao recebe resposta alguma — nem sucesso, nem erro — mesmo
// esperando 100s; nenhum prazo "resolve" esse caso, entao o valor so precisa
// separar "lento mas vivo" de "definitivamente parado" sem prender a suite
// do emulador (12 figuras ja publicadas) atras de uma instalacao fora do ar.
const LOGIN_TIMEOUT_MS = 20_000

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

// O W-Access roda em HTTPS com certificado self-signed, dai o
// ignoreHTTPSErrors. E um ASP.NET WebForms — versao 4.210.8 confirmada, tela
// de login em Login.aspx.
//
// Duas coisas se comportam DIFERENTE aqui de proposito, e nao por descuido:
//   - um seletor que nao casa continua LANCANDO (mesma regra de disparar).
//     Seletor errado e bug de manual desatualizado, tem que derrubar a
//     captura pra alguem corrigir — nao pode virar um aviso silencioso, ou
//     uma seta do PDF acaba apontando pro lugar errado sem ninguem notar.
//   - servidor inalcancavel, ou login que nao completa dentro do prazo, NAO
//     lanca. Vira aviso e a suite do W-Access inteira e pulada dali, porque
//     a suite do emulador (12 figuras ja publicadas, funcionando) nao pode
//     ficar refem de uma instalacao do W-Access fora do ar — ver §8.4 do
//     spec e task-4-report.md, onde o servico de backend ficou fora do ar
//     durante todo o desenvolvimento desta suite (a propria tela de login
//     exibe "INVENZI W-ACCESS SERVICE NOT RUNNING") e o POST de login nunca
//     recebeu resposta, nem depois de 100s de espera.
async function suiteWxs(browser, env) {
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: ESCALA,
    ignoreHTTPSErrors: true
  })
  const page = await context.newPage()

  // waitUntil: 'load', nao 'networkidle' — confirmado que a Login.aspx
  // mantem trafego de fundo (o aviso "SERVICE NOT RUNNING" no rodape parece
  // sondar o backend periodicamente), entao 'networkidle' nunca assenta
  // mesmo quando a pagina em si carregou por completo. 'load' e o que o
  // relato de exploracao (task-4-report.md) confirmou que completa
  // normalmente mesmo com o servico de backend fora do ar.
  try {
    await page.goto(env.WXS_URL, { waitUntil: 'load', timeout: LOGIN_TIMEOUT_MS })
  } catch (erro) {
    console.warn(
      `[captura] W-Access inalcancavel em ${env.WXS_URL} (${erro.message}) — ` +
      'suite pulada, as capturas do emulador seguem normalmente.'
    )
    await context.close()
    return
  }

  // A tela de login e fotografada ANTES de preencher, para o manual mostrar
  // o campo vazio e nao a credencial de quem gerou.
  //
  // Os tres seletores abaixo sao CONFIRMADOS (ver task-4-report.md, obtidos
  // com page.$$eval sobre todos os <input> da Login.aspx real): a pagina e
  // WebForms e tem VARIOS input[type="text"] visiveis ao mesmo tempo (campos
  // ocultos de telemetria do cliente), entao um seletor generico por type
  // bateria em mais de um elemento e falharia por ambiguidade no modo
  // estrito do Playwright.
  await disparar(page, {
    url: null,
    arquivo: 'wxs-login.png'
  })

  await page.fill('#txt_Operator', env.WXS_USUARIO)
  await page.fill('#txt_Password', env.WXS_SENHA)
  await page.click('#btnBD_Login')

  // E aqui que o backend do W-Access pode travar: o handler de login faz
  // (aparentemente) uma chamada sincrona pro servico Windows que fala com o
  // banco/hardware, e se esse servico estiver fora do ar o POST nunca
  // recebe resposta — nem sucesso, nem erro, nenhum evento de rede novo.
  // Prazo limitado de proposito, ver LOGIN_TIMEOUT_MS.
  //
  // 'load', nao 'networkidle': confirmado nesta mesma execucao que a
  // Login.aspx nunca assenta em rede ociosa (ver comentario no goto()
  // acima) — um sucesso de login provavelmente troca de pagina (redirect
  // pos-autenticacao), o que dispara um novo evento 'load' de verdade; um
  // travamento nao dispara evento nenhum de qualquer forma, entao 'load'
  // deteta os dois casos sem correr o risco de nunca assentar numa pagina
  // pos-login que tambem tenha trafego de fundo.
  try {
    await page.waitForLoadState('load', { timeout: LOGIN_TIMEOUT_MS })
  } catch {
    console.warn(
      `[captura] login do W-Access nao completou em ${LOGIN_TIMEOUT_MS / 1000}s — ` +
      'servico de backend provavelmente fora do ar (ver task-4-report.md). ' +
      'wxs-pos-login.png, wxs-controladores.png e wxs-inicial.png ficam pendentes.'
    )
    await context.close()
    return
  }

  // A tela inicial depois do login, so para o manual mostrar que o acesso
  // deu certo antes de mandar navegar.
  await disparar(page, {
    url: null,
    arquivo: 'wxs-pos-login.png'
  })

  // NAO CONFIRMADO — o login nunca completou durante o desenvolvimento desta
  // suite (ver task-4-report.md), entao esta tela nunca foi vista de
  // verdade. Textos de menu chutados cobrindo ingles (idioma confirmado
  // desta instalacao, "OPERATOR:"/"PASSWORD:"/"LOGIN" na tela de login) e
  // portugues, caso outra instalacao esteja localizada. Na primeira execucao
  // real: se a regex nao casar, o Playwright lanca apontando qual — leia o
  // texto verdadeiro do link na tela e troque so a regex correspondente
  // abaixo, nao adicione mais alternativas "por garantia".
  // 'load' pelo mesmo motivo do wait pos-login: 'networkidle' nao assenta
  // nesta instalacao.
  await page.getByRole('link', { name: /devices|dispositivos/i }).first().click()
  await page.getByRole('link', { name: /controllers|controladores/i }).first().click()
  await page.waitForLoadState('load')

  await disparar(page, {
    url: null,
    arquivo: 'wxs-controladores.png'
  })

  // NAO CONFIRMADO, mesmo motivo acima. Abre o primeiro controlador da lista
  // pra chegar na tela de cadastro (wxs-inicial.png), onde a descricao
  // emulator_NN, o endereco e o BaseCommPort sao preenchidos pelo tecnico.
  // `table a` e um chute generico (a tabela de controladores provavelmente
  // usa <a> dentro de <table> pra abrir o registro, como e comum em grids do
  // WebForms) — na primeira execucao real, confirme isso olhando o DOM da
  // tela de controladores; se for outro elemento (linha clicavel, botao de
  // icone), troque so este seletor.
  await page.locator('table a').first().click()
  await page.waitForLoadState('load')

  await disparar(page, {
    url: null,
    arquivo: 'wxs-inicial.png'
  })

  await context.close()
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await mkdir(DESTINO, { recursive: true })
  const browser = await chromium.launch({ channel: 'chrome' })
  try {
    await suiteEmulador(browser)

    const env = await lerCredenciais()
    if (env) {
      await suiteWxs(browser, env)
    } else {
      console.log(
        '[captura] .captura.env ausente ou incompleto — suite do W-Access ' +
        'pulada. Copie .captura.env.exemplo e preencha.'
      )
    }
  } finally {
    await browser.close()
  }
}
