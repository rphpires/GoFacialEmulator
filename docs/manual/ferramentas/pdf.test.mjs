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
