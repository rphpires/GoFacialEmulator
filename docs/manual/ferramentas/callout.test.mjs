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
