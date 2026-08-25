import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, mkdir, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { montar } from './montar.mjs'

// cenario cria uma arvore de manual descartavel, para os testes nao
// dependerem do conteudo real que ainda esta sendo escrito.
async function cenario(capitulos, arquivos, imagens = []) {
  const raiz = await mkdtemp(join(tmpdir(), 'manual-'))
  await mkdir(join(raiz, 'conteudo', 'comum'), { recursive: true })
  await mkdir(join(raiz, 'ativos', 'css'), { recursive: true })
  await mkdir(join(raiz, 'ativos', 'img', 'manual'), { recursive: true })

  await writeFile(join(raiz, 'ativos', 'css', 'manual.css'), 'body{}')
  await writeFile(
    join(raiz, 'manifesto.json'),
    JSON.stringify({ alvos: { teste: { titulo: 'T', subtitulo: 'S', capitulos } } })
  )

  for (const [caminho, conteudo] of Object.entries(arquivos)) {
    await writeFile(join(raiz, 'conteudo', caminho), conteudo)
  }
  for (const nome of imagens) {
    await writeFile(join(raiz, 'ativos', 'img', 'manual', nome), 'png-de-mentira')
  }
  return raiz
}

test('capitulos entram na ordem do manifesto', async () => {
  const raiz = await cenario(
    ['comum/b.md', 'comum/a.md'],
    { 'comum/a.md': '# Segundo\n\ntexto a\n', 'comum/b.md': '# Primeiro\n\ntexto b\n' }
  )

  const { html } = await montar('teste', { raiz })

  assert.ok(html.indexOf('Primeiro') < html.indexOf('Segundo'),
    'a ordem do manifesto deve mandar, nao a ordem alfabetica')
})

test('capitulos sao numerados automaticamente', async () => {
  const raiz = await cenario(
    ['comum/a.md', 'comum/b.md'],
    { 'comum/a.md': '# Antes de comecar\n', 'comum/b.md': '# Instalar\n' }
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /1\.\s*Antes de comecar/)
  assert.match(html, /2\.\s*Instalar/)
})

test('sumario lista todos os capitulos', async () => {
  const raiz = await cenario(
    ['comum/a.md', 'comum/b.md'],
    { 'comum/a.md': '# Antes de comecar\n', 'comum/b.md': '# Instalar\n' }
  )

  const { html } = await montar('teste', { raiz })
  const sumario = html.slice(html.indexOf('id="sumario"'), html.indexOf('id="conteudo"'))

  assert.match(sumario, /Antes de comecar/)
  assert.match(sumario, /Instalar/)
})

test('capitulo declarado e ausente do disco falha o build', async () => {
  const raiz = await cenario(['comum/nao-existe.md'], {})

  await assert.rejects(
    () => montar('teste', { raiz }),
    /nao-existe\.md/,
    'a mensagem precisa dizer qual capitulo faltou'
  )
})

test('figura existente vira img, figura ausente vira placeholder e pendencia', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    {
      'comum/a.md':
        '# Instalar\n\n' +
        '![Tela que existe](img/manual/existe.png)\n\n' +
        '![Tela do wizard do Docker Desktop](img/manual/falta.png)\n'
    },
    ['existe.png']
  )

  const { html, pendencias } = await montar('teste', { raiz })

  assert.match(html, /<img[^>]+existe\.png/)
  assert.doesNotMatch(html, /<img[^>]+falta\.png/)
  assert.match(html, /figura-pendente/)
  assert.match(html, /Tela do wizard do Docker Desktop/)

  assert.equal(pendencias.length, 1)
  assert.equal(pendencias[0].alt, 'Tela do wizard do Docker Desktop')
  assert.match(pendencias[0].caminho, /falta\.png$/)
})

test('figuras sao numeradas e o placeholder tambem conta', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    {
      'comum/a.md':
        '# Instalar\n\n![Primeira](img/manual/existe.png)\n\n![Segunda](img/manual/falta.png)\n'
    },
    ['existe.png']
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /Figura 1\.1/)
  assert.match(html, /Figura 1\.2/)
})

test('alvo inexistente falha com mensagem clara', async () => {
  const raiz = await cenario([], {})
  await assert.rejects(() => montar('nao-existe', { raiz }), /nao-existe/)
})

test('figura com titulo entre aspas e reconhecida mesmo existindo no disco', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    {
      'comum/a.md':
        '# Instalar\n\n' +
        '![Tela do instalador](img/manual/existe.png "Tela do instalador do Docker Desktop")\n'
    },
    ['existe.png']
  )

  const { html, pendencias } = await montar('teste', { raiz })

  assert.match(html, /<img[^>]+existe\.png/)
  assert.doesNotMatch(html, /figura-pendente/)
  assert.equal(pendencias.length, 0,
    'o titulo entre aspas nao pode ser confundido com parte do caminho do arquivo')
})

test('capitulo sem cabecalho de titulo falha o build', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    { 'comum/a.md': 'texto solto, sem "# Titulo" nenhum\n' }
  )

  await assert.rejects(
    () => montar('teste', { raiz }),
    /comum\/a\.md/,
    'a mensagem precisa dizer qual capitulo ficou sem titulo'
  )
})
