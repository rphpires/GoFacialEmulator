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

  // O numero do capitulo vive na faixa de abertura, separado do titulo,
  // porque os dois sao desenhados de formas diferentes.
  assert.match(html, /abertura-numero">01<[\s\S]*?<h1>Antes de comecar<\/h1>/)
  assert.match(html, /abertura-numero">02<[\s\S]*?<h1>Instalar<\/h1>/)
  assert.match(html, /sumario-num">1<[\s\S]*?Antes de comecar/)
  assert.match(html, /sumario-num">2<[\s\S]*?Instalar/)
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

test('subsecoes entram no sumario e na abertura do capitulo', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    { 'comum/a.md': '# Instalar\n\n## Baixar\n\ntexto\n\n## Rodar\n\ntexto\n' }
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /class="sumario-subs"/)
  assert.match(html, /href="#cap-1-s1"/)
  assert.match(html, /href="#cap-1-s2"/)
  assert.match(html, /neste-capitulo[\s\S]*?Baixar[\s\S]*?Rodar/)
})

test('secao numerada vira passo com marcador de acao', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    { 'comum/a.md': '# Roteiro\n\n## 1. Abrir o console\n\ntexto\n' }
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /class="secao secao-passo"/)
  assert.match(html, /secao-numero">1<\/span><span>Abrir o console</,
    'o numero sai do titulo e vira marcador; o texto fica sem ele')
})

test('linhas do roteiro viram faixas rotuladas', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    {
      'comum/a.md':
        '# Roteiro\n\n' +
        '**Faça:** abra o console.\n\n' +
        '**Tem que acontecer:** a lista carrega.\n\n' +
        '**Se não acontecer:** veja Problemas.\n'
    }
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /passo-linha passo-faca[\s\S]*?abra o console/)
  assert.match(html, /passo-linha passo-esperado[\s\S]*?a lista carrega/)
  assert.match(html, /passo-linha passo-desvio[\s\S]*?veja Problemas/)
  assert.doesNotMatch(html, /<p><strong>Faça:<\/strong>/,
    'a linha de passo nao pode sobrar tambem como paragrafo comum')
})

test('callout vira aviso com rotulo', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    { 'comum/a.md': '# Portas\n\n> [!atencao]\n> a porta pode ficar fora da faixa.\n' }
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /class="aviso aviso-atencao"/)
  assert.match(html, /aviso-rotulo">Atenção</)
  assert.doesNotMatch(html, /\[!atencao\]/, 'o marcador nao pode vazar para o PDF')
})

test('marcador de aviso desconhecido falha o build', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    { 'comum/a.md': '# Portas\n\n> [!cuidado]\n> texto\n' }
  )

  await assert.rejects(() => montar('teste', { raiz }), /cuidado/)
})

test('bloco de codigo com arquivo na info string ganha barra', async () => {
  const raiz = await cenario(
    ['comum/a.md'],
    { 'comum/a.md': '# Portas\n\n```yaml docker-compose.yml\nports: []\n```\n' }
  )

  const { html } = await montar('teste', { raiz })

  assert.match(html, /bloco-barra"><span>docker-compose\.yml<\/span>/)
  assert.match(html, /bloco-barra-acao">yaml</)
})
