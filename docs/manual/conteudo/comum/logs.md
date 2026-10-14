# Onde estão os logs

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

Cada emulador pode gravar o próprio log, separado do log geral. Ele
registra quem conectou (endereço e porta de origem — útil para descobrir
dois gerenciadores falando com o mesmo emulador), cada requisição recebida
com os dados principais do objeto (usuário, cartão, evento) e cada evento
que o emulador enviou ao gerenciador.

Para ligar, abra a gaveta de detalhes do dispositivo, aba Configurações, e
marque **Gravar log de eventos**. Vale na hora, com o emulador rodando ou
parado. Com o log ligado aparecem dois links:

- **Ver log** — abre o arquivo mais recente no navegador.
- **Baixar logs (.zip)** — baixa todos os arquivos daquele emulador.

Os arquivos ficam em `sistema/logs/devices/` (`device-<id>.log` é o atual;
`device-<id>.1.log` a `.7.log` são os anteriores). São guardados no máximo
8 arquivos de 3MB por emulador; o mais antigo é apagado ao passar disso.

**Desligar o log apaga os arquivos daquele emulador** — a tela pede
confirmação. Baixe o zip antes se quiser guardar. Excluir o emulador também
apaga os arquivos.

## O log de instalação

`instalacao.log` só registra o que aconteceu durante o instalador — é o
primeiro lugar a olhar quando o instalador falha antes mesmo da aplicação
subir.
