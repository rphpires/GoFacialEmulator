# Onde estao os logs

Cada pacote grava os logs em um lugar um pouco diferente. Esta tabela
mostra onde procurar em cada um.

| Pacote | Aplicação | Banco | Instalação |
|---|---|---|---|
| Docker | `sistema/logs/trace.log` | `docker compose -f sistema/docker-compose.yml logs postgres` | `sistema/logs/instalacao.log` |
| Windows | `sistema\logs\trace.log` | `sistema\logs\postgres.log` | `sistema\logs\instalacao.log` |
| Linux/WSL | `sistema/logs/trace.log` e `sistema/logs/app.out` | `/var/log/postgresql/` | `sistema/logs/instalacao.log` |

## O log da aplicação

`trace.log` é um arquivo de texto simples, linha por linha, com tudo que
a aplicação registrou. Ao lado dele fica `trace.html` — o mesmo conteúdo,
colorido por tipo de mensagem, feito para abrir direto no navegador em
vez de num editor de texto. Prefira o `trace.html` quando estiver
procurando um erro à mão; use o `trace.log` quando precisar copiar um
trecho ou mandar para outra pessoa.

No pacote Linux/WSL existe ainda `app.out`, a saída bruta do processo —
útil quando a aplicação nem chega a escrever no `trace.log`, por exemplo
se não conseguiu iniciar.

## O log de um dispositivo específico

O log de um dispositivo não fica junto do log geral da aplicação. Ele sai
pela gaveta de detalhes daquele dispositivo: na aba Configurações, marque
**Gravar log de eventos** e clique em **Salvar** para ligar o registro; a
partir daí, os eventos desse dispositivo passam a ser gravados em
arquivo. Veja o capítulo Conhecer o console, na seção sobre a gaveta de
detalhes, para a explicação completa da tela.

## O log de instalação

`instalacao.log` só registra o que aconteceu durante o instalador — é o
primeiro lugar a olhar quando o instalador falha antes mesmo da aplicação
subir.
