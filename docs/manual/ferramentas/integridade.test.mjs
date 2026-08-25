import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile, readdir } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const RAIZ = resolve(dirname(fileURLToPath(import.meta.url)), '..')

// Todo capitulo declarado no manifesto tem de existir, e todo capitulo no
// disco tem de estar declarado em ao menos um alvo. Capitulo orfao e
// trabalho escrito que ninguem le.
test('manifesto e disco concordam', async () => {
  const manifesto = JSON.parse(await readFile(join(RAIZ, 'manifesto.json'), 'utf8'))

  const declarados = new Set()
  for (const alvo of Object.values(manifesto.alvos)) {
    for (const c of alvo.capitulos) declarados.add(c)
  }

  const noDisco = new Set()
  for (const pasta of ['comum', 'docker', 'windows', 'linux']) {
    let arquivos = []
    try {
      arquivos = await readdir(join(RAIZ, 'conteudo', pasta))
    } catch {
      continue
    }
    for (const a of arquivos) {
      if (a.endsWith('.md')) noDisco.add(`${pasta}/${a}`)
    }
  }

  for (const d of declarados) {
    assert.ok(noDisco.has(d), `declarado no manifesto e ausente do disco: ${d}`)
  }
  for (const d of noDisco) {
    assert.ok(declarados.has(d), `capitulo orfao, nao declarado em nenhum alvo: ${d}`)
  }
})

test('nenhum capitulo escreve numeracao a mao', async () => {
  for (const pasta of ['comum', 'docker', 'windows', 'linux']) {
    let arquivos = []
    try {
      arquivos = await readdir(join(RAIZ, 'conteudo', pasta))
    } catch {
      continue
    }
    for (const a of arquivos.filter((x) => x.endsWith('.md'))) {
      const texto = await readFile(join(RAIZ, 'conteudo', pasta, a), 'utf8')
      const titulo = texto.split(/\r?\n/).find((l) => l.startsWith('# '))
      assert.ok(titulo, `${pasta}/${a} nao tem titulo de capitulo`)
      assert.doesNotMatch(
        titulo,
        /^#\s+\d+[.)]/,
        `${pasta}/${a} escreve o numero do capitulo a mao — a numeracao e gerada`
      )
    }
  }
})

test('os tres alvos existem no manifesto', async () => {
  const manifesto = JSON.parse(await readFile(join(RAIZ, 'manifesto.json'), 'utf8'))
  for (const alvo of ['docker', 'windows', 'linux']) {
    assert.ok(manifesto.alvos[alvo], `alvo ausente: ${alvo}`)
    assert.ok(manifesto.alvos[alvo].capitulos.length > 0, `alvo vazio: ${alvo}`)
  }
})
