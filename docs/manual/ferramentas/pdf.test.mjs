import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, writeFile, readFile, stat } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { imprimir } from './pdf.mjs'

// O teste imprime um HTML minimo de duas paginas. Nao depende do conteudo
// real dos manuais, que ainda esta sendo escrito.
test('imprime um PDF de verdade e conta as paginas', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'manual-pdf-'))
  const html = join(dir, 'teste.html')
  const pdf = join(dir, 'teste.pdf')

  await writeFile(
    html,
    `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8">
     <style>@page{size:A4;margin:20mm}.p{break-after:page}</style></head>
     <body><div class="p">Pagina um</div><div>Pagina dois</div></body></html>`,
    'utf8'
  )

  const { paginas } = await imprimir(html, pdf)

  assert.ok(paginas >= 2, `esperava ao menos 2 paginas, veio ${paginas}`)

  const bytes = await readFile(pdf)
  assert.equal(bytes.subarray(0, 4).toString('latin1'), '%PDF',
    'o arquivo precisa ser um PDF de verdade')

  const info = await stat(pdf)
  assert.ok(info.size > 1000, 'PDF suspeitosamente pequeno')
})

// paged.js pagina documentos grandes em varios ciclos, acrescentando paginas
// ao DOM aos poucos. Um detector que confia no primeiro ".pagedjs_page" que
// aparece no DOM pega uma paginacao parcial — o fixture de duas paginas do
// teste acima nao pega esse bug porque paginacao curta cabe num unico ciclo.
// Este teste usa um documento grande o bastante para atravessar varios
// ciclos e prova o total REAL, nao so "mais de uma pagina".
test('conta o total real de paginas em documentos que paginam em varios ciclos', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'manual-pdf-grande-'))
  const html = join(dir, 'grande.html')
  const pdf = join(dir, 'grande.pdf')

  // Cada paragrafo, exceto o ultimo, forca uma quebra de pagina depois de
  // si. Isso torna o numero de paginas conhecido de antemao (N paragrafos
  // = N paginas), entao a assercao pode exigir um valor exato em vez de só
  // "aumentou" — um numero exato e o unico jeito de provar que a contagem
  // e a final, e nao uma parcial que por acaso e maior que 1.
  const N = 200
  const paragrafos = Array.from({ length: N }, (_, i) =>
    `<div class="${i < N - 1 ? 'p' : ''}">Paragrafo ${i + 1}</div>`
  ).join('\n')

  await writeFile(
    html,
    `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8">
     <style>@page{size:A4;margin:20mm}.p{break-after:page}</style></head>
     <body>${paragrafos}</body></html>`,
    'utf8'
  )

  const { paginas } = await imprimir(html, pdf)

  assert.equal(paginas, N,
    `esperava exatamente ${N} paginas (uma por paragrafo forcado), veio ${paginas} — ` +
    'sinal de contagem feita antes do paged.js terminar de paginar')

  // Confere o PDF gravado em disco, independente do numero que imprimir()
  // devolveu: cada pagina do PDF e um objeto "/Type /Page" (o "(?!s)" evita
  // casar com o array "/Type /Pages", que e outra coisa). Se o arquivo
  // tivesse sido escrito com menos paginas que o total real, essa contagem
  // pegaria mesmo que "paginas" estivesse certo por coincidencia.
  const bytes = await readFile(pdf, 'latin1')
  const objetosDePagina = bytes.match(/\/Type\s*\/Page(?!s)/g) ?? []
  assert.equal(objetosDePagina.length, N,
    `o PDF gravado em disco deveria ter ${N} objetos de pagina, tem ${objetosDePagina.length}`)
})
