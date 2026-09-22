// Captura as telas do emulador e do W-Access para os manuais.
//
// Passo manual e deliberado: exige ambiente vivo (banco, emulador, W-Access).
// O build dos PDFs NAO chama este script — ele monta a partir dos PNGs
// versionados, para que qualquer estacao gere release sem subir nada.
import { mkdir, readFile, access } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { abrirNavegador } from './navegador.mjs'

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
// Medido nesta instalacao (192.168.1.138, IIS em RPH-SRV): a propria
// Login.aspx leva ~35s para completar o 'load' — dois redirects (o segundo
// injeta o id de sessao WebForms no caminho) e so entao o HTML. 20s cortava
// a navegacao antes mesmo da pagina existir, e o sintoma era enganoso:
// aparecia como servidor inalcancavel, quando o servidor so era lento.
// 90s da folga de 2.5x sobre o medido sem deixar de ser finito.
const LOGIN_TIMEOUT_MS = 90_000

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
  let page = await context.newPage()

  // insistir repete um passo ate ele passar ou o prazo acabar, com aba nova
  // a cada rodada.
  //
  // Numa estacao que roda Docker o Chromium aborta navegacoes em voo com
  // net::ERR_NETWORK_CHANGED toda vez que uma interface de rede aparece ou
  // some — e cada container que sobe cria um veth novo. Confirmado com
  // `docker events` na estacao de desenvolvimento: ha "network connect" e
  // "container start" o tempo todo, enquanto o mesmo endereco respondia
  // normalmente via curl. E falha de ambiente, nao do servidor, e a janela de
  // ~35s que cada tela do W-Access leva para carregar da azar com facilidade.
  //
  // Insistir por TEMPO, e nao por numero de tentativas: um aborto volta em
  // ~5s, um carregamento que da certo leva ~35s. Contar tentativas gastaria o
  // orcamento em meia duzia de abortos rapidos sem nunca dar ao servidor uma
  // janela inteira.
  //
  // A aba e recriada a cada rodada porque reaproveitar a que ficou na pagina
  // de erro interna do Chromium faz a navegacao seguinte ser cancelada por
  // ela, e o sintoma vira "interrupted by another navigation to
  // chrome-error://chromewebdata/", que esconde o erro verdadeiro. A pausa
  // antes de tentar de novo evita disparar um goto por cima de outro.
  const PRAZO_PADRAO_MS = 5 * 60_000
  async function insistir (rotulo, passo, prazoMs = PRAZO_PADRAO_MS) {
    const limite = Date.now() + prazoMs
    let ultimoErro = null
    let rodada = 0
    while (Date.now() < limite) {
      rodada += 1
      try {
        return await passo()
      } catch (erro) {
        ultimoErro = erro
        console.warn(
          `[captura] ${rotulo}: tentativa ${rodada} falhou ` +
          `(${erro.message.split('\n')[0]})`
        )
        await page.close().catch(() => {})
        page = await context.newPage()
        await page.waitForTimeout(4000)
      }
    }
    throw ultimoErro
  }

  // Etapa 1: chegar na tela de login.
  //
  // waitUntil: 'load', nao 'networkidle' — confirmado que a Login.aspx mantem
  // trafego de fundo (o aviso do rodape sonda o backend periodicamente),
  // entao 'networkidle' nunca assenta mesmo com a pagina ja carregada.
  //
  // Servidor fora do ar NAO lanca: vira aviso e a suite do W-Access inteira e
  // pulada dali, porque a suite do emulador (12 figuras publicadas) nao pode
  // ficar refem de uma instalacao do W-Access parada.
  try {
    await insistir('abrir o W-Access', async () => {
      await page.goto(env.WXS_URL, { waitUntil: 'load', timeout: LOGIN_TIMEOUT_MS })
      if (page.url().startsWith('chrome-error')) throw new Error('navegacao abortada')
    })
  } catch (erro) {
    console.warn(
      `[captura] W-Access inalcancavel em ${env.WXS_URL} (${erro.message.split('\n')[0]}) — ` +
      'suite pulada, as capturas do emulador seguem normalmente.'
    )
    await context.close()
    return
  }

  // O rodape da Login.aspx anuncia quando o servico Windows do W-Access esta
  // parado. Com ele parado nenhuma figura desta suite presta: a de login sai
  // com um aviso vermelho de erro que nao tem nada a ver com o passo que o
  // manual ensina, e o POST de login nao recebe resposta, entao as tres
  // seguintes nem existem. Figura com erro estampado e pior que figura
  // pendente — ninguem revisando o PDF adivinha que aquele vermelho nao devia
  // estar ali.
  if (/SERVICE NOT RUNNING/i.test(await page.content())) {
    console.warn(
      '[captura] W-Access respondeu, mas o servico esta parado ' +
      '("INVENZI W-ACCESS SERVICE NOT RUNNING" no rodape da tela de login) — ' +
      'suite pulada. Inicie o servico no servidor e rode de novo; nenhuma ' +
      'figura do W-Access sai util com ele parado.'
    )
    await context.close()
    return
  }

  // A tela de login e fotografada ANTES de preencher, para o manual mostrar o
  // campo vazio e nao a credencial de quem gerou.
  //
  // Os tres seletores sao confirmados na Login.aspx real: a pagina e WebForms
  // e tem varios input[type="text"] visiveis ao mesmo tempo (campos ocultos de
  // telemetria do cliente), entao um seletor generico por type bateria em mais
  // de um elemento e falharia por ambiguidade no modo estrito do Playwright.
  await disparar(page, {
    url: null,
    arquivo: 'wxs-login.png'
  })

  // Etapa 2: entrar. Repete o passo inteiro (abrir, preencher, submeter)
  // porque um aborto de rede no meio deixa a aba na pagina de erro, e nao
  // adianta so reenviar o formulario que nao existe mais.
  await insistir('login no W-Access', async () => {
    if (!page.url().includes('Login.aspx')) {
      await page.goto(env.WXS_URL, { waitUntil: 'load', timeout: LOGIN_TIMEOUT_MS })
    }
    await page.fill('#txt_Operator', env.WXS_USUARIO)
    await page.fill('#txt_Password', env.WXS_SENHA)
    await page.click('#btnBD_Login')
    await page.waitForLoadState('load', { timeout: LOGIN_TIMEOUT_MS })
    await page.waitForTimeout(4000)
    if (page.url().startsWith('chrome-error')) throw new Error('navegacao abortada')
    if (page.url().includes('Login.aspx')) throw new Error('continuou na tela de login')
  })

  // A tela inicial depois do login (Default.aspx, que carrega a lista de
  // cardholders num iframe), so para o manual mostrar que o acesso deu certo.
  await disparar(page, {
    url: null,
    arquivo: 'wxs-pos-login.png'
  })

  // O W-Access carrega o id da sessao NO CAMINHO da URL
  // (.../W-Access/(S(<id>))/Default.aspx), e nao so em cookie. Toda tela
  // seguinte precisa ser aberta debaixo desse mesmo prefixo, senao a sessao
  // se perde e o servidor devolve a tela de login de novo.
  const base = page.url().slice(0, page.url().lastIndexOf('/') + 1)

  // Etapa 3: a lista de controladores.
  //
  // O caminho pela interface e SYSTEM > HARDWARE SETTINGS > Devices, que abre
  // CfgSYLocalitiesFullHierarchy.aspx. Vamos direto na URL em vez de clicar os
  // tres niveis: o menu e um TreeView de WebForms que faz um postback por
  // nivel, e cada postback e mais uma navegacao exposta ao aborto de rede
  // descrito em insistir(). A URL e estavel, o menu nao precisa ser encenado.
  //
  // A arvore abre fechada, so com as localidades. A busca por "emulator"
  // expande exatamente os controladores do emulador — que e o que o capitulo
  // manda o leitor procurar.
  //
  // O clique vai pelo DOM (element.click()) e nao pelo mouse: durante os
  // postbacks o W-Access cobre a tela com #ctl00_DivProgress, um overlay que
  // intercepta ponteiro e faz o clique real expirar. Nao ha nada de visual a
  // validar no botao de busca, so o postback que ele dispara.
  async function abrirListaDeControladores () {
    await page.goto(base + 'CfgSYLocalitiesFullHierarchy.aspx', {
      waitUntil: 'load',
      timeout: LOGIN_TIMEOUT_MS
    })
    if (page.url().startsWith('chrome-error')) throw new Error('navegacao abortada')
    await page.$eval('#ctl00_ContentPlaceHolder1_txt_Search', (campo, valor) => {
      campo.value = valor
    }, 'emulator')
    await page.$eval('#ctl00_ContentPlaceHolder1_btn_Search', (botao) => botao.click())

    // Espera pelo resultado, e nao por tempo fixo: os nos da arvore sao
    // <span class="rtIn"> do RadTreeView, e os controladores do emulador
    // aparecem como "CTRL_emulator_NNN - emulator_NN [host:porta]".
    await page.waitForFunction(() => {
      return [...document.querySelectorAll('span.rtIn')]
        .some((no) => /^CTRL_emulator/i.test((no.innerText || '').trim()))
    }, null, { timeout: 90_000 })
    await page.waitForTimeout(1500)
  }

  await insistir('lista de controladores', abrirListaDeControladores)

  await disparar(page, {
    url: null,
    arquivo: 'wxs-controladores.png'
  })

  // Etapa 4: o cadastro de um controlador.
  //
  // Clicar no no NAO navega: o formulario do controlador carrega por AJAX no
  // painel da direita, na mesma URL. Por isso a espera e pelo conteudo do
  // painel — o rotulo "ID: NNN", que so existe com um controlador carregado —
  // e nao por evento de navegacao, que nunca vem.
  //
  // force: true porque o mesmo overlay de progresso (#ctl00_DivProgress) pode
  // estar por cima quando o clique acontece.
  //
  // O primeiro CTRL_emulator da lista serve: a figura existe para mostrar
  // ONDE ficam a descricao comecando com `emulator`, o endereco e o
  // BaseCommPort, nao para documentar um controlador especifico.
  await insistir('cadastro do controlador', async () => {
    // Uma tentativa que falhou trocou a aba, e a aba nova nasce em branco —
    // sem a lista, sem a busca. Refazer o caminho antes de procurar o no e o
    // que torna esta etapa capaz de se recuperar sozinha.
    const no = page.locator('span.rtIn').filter({ hasText: /^CTRL_emulator/ }).first()
    if (await no.count() === 0) await abrirListaDeControladores()
    await no.scrollIntoViewIfNeeded()
    await no.click({ force: true })
    await page.waitForSelector(
      '#ctl00_ContentPlaceHolder1_ControlLocalController_lbl_ID',
      { state: 'visible', timeout: 90_000 }
    )
    await page.waitForTimeout(1500)
  })

  await disparar(page, {
    url: null,
    arquivo: 'wxs-inicial.png'
  })

  await context.close()
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await mkdir(DESTINO, { recursive: true })
  const browser = await abrirNavegador()
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
