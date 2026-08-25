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
