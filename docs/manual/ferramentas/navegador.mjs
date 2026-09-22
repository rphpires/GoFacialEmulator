// Abertura do navegador, compartilhada pela captura e pela impressao do PDF.
import { chromium } from 'playwright'

// abrirNavegador prefere o Chrome ja instalado na maquina (channel: 'chrome')
// e cai no Chromium que vem com o Playwright quando ele nao existe.
//
// O Chrome real continua sendo a primeira escolha pelo motivo original: e o
// navegador que ja esta na estacao de quem gera o release, entao nao ha 150 MB
// de download de browser so para montar um PDF.
//
// A queda para o Chromium empacotado existe porque a estacao Linux de
// desenvolvimento nao tem Chrome — e sem ela tanto a captura quanto o PDF
// morriam no launch com "Run npx playwright install chrome", que e um erro
// sobre o ambiente disfarcado de erro do build.
export async function abrirNavegador(opcoes = {}) {
  try {
    return await chromium.launch({ ...opcoes, channel: 'chrome' })
  } catch {
    console.warn(
      '[navegador] Chrome nao encontrado nesta maquina — usando o Chromium do ' +
      'Playwright. O resultado sai equivalente; se alguma fonte destoar, ' +
      'instale o Chrome e rode de novo.'
    )
    return await chromium.launch(opcoes)
  }
}
