# Logs por emulador

Data: 2026-10-14
Status: aprovado para planejamento

## Problema

A gaveta de detalhes e a tabela de dispositivos têm um checkbox **Gravar
log de eventos**. Ele grava `log_enabled` em `service.devices`, aparece no
stream e na UI — e nenhum emulador lê essa flag. Não existe arquivo de log
por dispositivo; o manual (`docs/manual/conteudo/comum/logs.md`) promete
um que não existe.

O único log hoje é o `trace.log` global, onde os `[REQ]/[RES]` de todos os
emuladores se misturam e não trazem detalhe dos objetos.

O caso que motivou este trabalho: dois gerenciadores conversando com o
mesmo emulador ao mesmo tempo. Para enxergar isso é preciso ver, por
emulador, quem conectou (IP:porta de origem) e o que cada um fez.

## Objetivo

Com o checkbox ligado, cada emulador grava seu próprio arquivo de log com:

- conexões TCP abertas e fechadas, com o endpoint de origem;
- cada requisição recebida, com origem, operação e detalhe dos objetos;
- cada evento que o emulador envia ao gerenciador.

O operador abre o log mais recente no navegador e baixa um zip com todos
os arquivos daquele emulador, sem acessar o servidor.

## Decisões

Tomadas com o usuário antes do design:

- **Abordagem:** pacote próprio `internal/devicelog` com rotação própria e
  middleware gin genérico. Sem dependência nova (descartado
  `lumberjack`). Sem resumo escrito à mão em cada handler (pode ser
  somado depois, nas operações mais importantes).
- **Formato:** texto puro `.log`, uma linha por evento, legível direto no
  navegador. Descartados JSON Lines e página visualizadora.
- **Detalhe dos objetos:** resumo com campos-chave + corpo truncado em
  2KB; imagens e base64 grande viram `<image/jpeg 48KB>`.
- **Retenção:** no máximo 8 arquivos de 3MB por emulador (24MB).
- **Liga/desliga com o emulador rodando.** A regra atual "só com o
  emulador parado" sai, na gaveta e na tabela.
- **Desligar apaga os arquivos** daquele emulador. A UI confirma antes.
- **Excluir o emulador** também apaga os arquivos.
- O `trace.log` global continua como está, inclusive os `[REQ]/[RES]`.

## Armazenamento e rotação

- Pasta: `logs/devices/`, dentro do `logs/` da aplicação (mesma raiz do
  `trace.log`), válida em todos os pacotes.
- Nomes por ID do dispositivo, nunca pelo nome (nome muda e pode ter
  caractere inválido):
  - ativo: `device-<id>.log` — sempre o mais recente;
  - rotacionados: `device-<id>.1.log` … `device-<id>.7.log`, `.1` o mais
    novo.
- Rotação por tamanho: ao passar de 3MB, apaga `.7`, desloca
  `.6→.7` … `.1→.2`, `ativo→.1`, abre novo ativo.
- Cada arquivo novo começa com cabeçalho, para fazer sentido sozinho no
  zip:
  `# device 12 "Portaria A" hikvision porta 4012 — aberto 2026-10-14 10:00:00`
- Limite de tamanho e quantidade são parâmetros do logger (3MB e 8 por
  padrão), para os testes usarem valores pequenos.

### Componentes

- `devicelog.Logger` — um por emulador. Mutex, flag `enabled` atômica,
  arquivo aberto, tamanho atual. Desligado, toda chamada é no-op e o
  arquivo fica fechado.
- `devicelog.Registry` — vive no `Manager`, mapa `id → *Logger`.
  - `Get(id, info)` devolve (criando se preciso) o logger do dispositivo.
  - `SetEnabled(id, on)` liga ou desliga; desligar fecha e apaga os
    arquivos.
  - `Remove(id)` desliga, apaga os arquivos e tira do mapa (usado por
    `DeleteDevice`).
- Ao iniciar, o emulador recebe o logger do registry, já com a flag
  vinda do banco (`device.LogEnabled`).
- `Manager.UpdateDeviceSettings` grava no banco e chama
  `registry.SetEnabled(id, on)` — vale com o emulador rodando ou parado.
- Escrita síncrona sob mutex. Volume é baixo; não se perde linha num
  crash.

## O que é capturado

Formato geral da linha: `AAAA-MM-DD HH:MM:SS.mmm TIPO ...` — cada linha
leva data e hora completas (ex.: `2026-10-14 10:02:03.120`). Tipos:
`CONN`, `IN`, `OUT`, `CONECTADO`, `DESCONECTADO`, `START`, `STOP`, `LOG`.

### Conexões TCP

Hook `ConnState` do `http.Server` de cada emulador:

```
2026-10-14 10:02:01.002 CONN 10.0.0.5:51234 aberta
2026-10-14 10:05:13.441 CONN 10.0.0.5:51234 fechada (37 reqs, 3m12s)
```

Com dois gerenciadores, dois IPs se alternam no log.

Premissa: no deploy Docker as portas `4000-4399` são publicadas via
iptables DNAT, que preserva o IP de origem. Validar na verificação
manual.

### Requisições recebidas

Middleware compartilhado `devicelog.Gin(logger)`, plugado no router do
Hikvision e do Dahua ao lado do middleware `[REQ]/[RES]` atual.

- Log desligado → o middleware retorna sem ler nada.
- Grava: origem (`RemoteAddr`), usuário (extraído do header
  Digest/Basic — nunca o header inteiro), método, rota + query, status,
  duração, tamanhos.
- **Campos-chave:** varre JSON, XML, form e query atrás de uma lista fixa
  (`employeeNo`, `name`, `cardNo`, `UserID`, `CardName`, `CardNo`,
  `major`, `minor`, `searchID`, `faceLibType`, …), sem diferenciar
  maiúsculas. No máximo 6 campos; valor cortado em 40 caracteres.
- **Corpos de requisição e resposta:** compactados em uma linha,
  truncados em 2KB depois da sanitização:
  - string base64 longa (> 256 caracteres) → `<base64 48KB>`;
  - parte binária de multipart ou corpo não-texto → `<image/jpeg 48KB>`
    (tipo + tamanho);
  - campos `password`/`pwd` → `***`.
- O corpo lido é devolvido ao handler (`io.NopCloser(bytes.NewReader)`).
  Corpo acima de 2MB não é copiado: registra só o tamanho e o handler
  recebe o stream original.
- A resposta é capturada por um `ResponseWriter` que guarda os primeiros
  2KB e conta o total.

Exemplo:

```
2026-10-14 10:02:03.120 IN  10.0.0.5:51234 user=admin PUT /ISAPI/AccessControl/UserInfo/Modify?format=json -> 200 12ms | employeeNo=123 name="Ana"
    > {"UserInfo":{"employeeNo":"123","name":"Ana",...}}
    < {"statusCode":1,"statusString":"OK"}
```

### Streams longos

`alertStream` (Hikvision) e `eventManager.cgi?action=attach` (Dahua) não
terminam como requisição comum:

- linha `CONECTADO` ao abrir, com origem;
- cada evento empurrado no stream vira linha `OUT`;
- linha `DESCONECTADO` ao fechar, com duração e quantidade de eventos;
- o corpo da resposta do stream não é capturado.

### Saídas para o gerenciador

Chamadas explícitas em `sendEventToRemoteServer`, `sendDoorEvent`
(ambos), `sendOnlineEventToServer`, `sendTextEventToServer` e
`pushStreamEvent`: URL de destino, resumo do evento (campos-chave),
status ou erro, duração.

```
2026-10-14 10:02:05.004 OUT -> http://10.0.0.5:8000/event POST major=5 minor=75 cardNo=999 -> 200 30ms
```

### Ciclo de vida

Linhas `START` e `STOP` do emulador e `LOG ligado`.

## Endpoints web

No grupo `/api/devices` existente. O console não tem autenticação; o log
fica tão exposto quanto o resto da tela.

| Rota | Comportamento |
|---|---|
| `GET /api/devices/:id/log` | `device-<id>.log` como `text/plain; charset=utf-8`, `Content-Disposition: inline`, `Cache-Control: no-store`. 404 se log desligado ou arquivo inexistente. |
| `GET /api/devices/:id/log.zip` | Zip montado em memória com os até 8 arquivos, sob o lock do logger (sem pegar rotação no meio). Nome `device-<id>-logs-AAAAMMDD-HHMM.zip`. 404 se não houver arquivos. |

- O caminho é montado a partir do `:id` numérico; nada vindo da
  requisição entra no caminho.
- 400 para id inválido; 500 com linha no `trace.log` se a leitura falhar.

## UI

Gaveta de detalhes → aba Configurações → painel "Emulador":

- Checkbox **Gravar log de eventos** habilitado com o emulador em
  qualquer estado. Mesmo na coluna Log da tabela.
- Ao desmarcar: `confirm("Desligar apaga os logs deste emulador. Continuar?")`.
  Cancelar mantém ligado. Vale na gaveta e na tabela.
- Abaixo do checkbox, só com log ligado:
  - **Ver log** — `<a href="/api/devices/<id>/log" target="_blank">`;
  - **Baixar logs (.zip)** — `<a href="/api/devices/<id>/log.zip" download>`.
- Texto de ajuda: *"Últimos 8 arquivos de 3MB. Desligar apaga os arquivos."*
- Estado vem do stream (`log_enabled`), como hoje: os links aparecem e
  somem sozinhos.

## Erros

O log nunca derruba o emulador nem altera a resposta ao gerenciador.

- Falha ao abrir ou gravar (disco cheio, permissão): uma linha `ERROR` no
  `trace.log`; o logger entra em estado "falhou" e para de tentar até o
  próximo liga/desliga ou restart do emulador.
- Falha na rotação: continua no arquivo atual; tenta de novo na próxima
  escrita que passar do limite.
- Desligar durante uma escrita: o mutex garante que o arquivo só fecha e
  é apagado depois da linha terminar.
- Corpo que não parseia: grava o texto truncado, sem campos-chave. Não é
  erro.

## Manual

Reescrever a seção "O log de um dispositivo específico" de
`docs/manual/conteudo/comum/logs.md`: pasta `logs/devices/`, nomes,
rotação 8×3MB, links Ver log / Baixar zip, desligar apaga.

## Testes

Padrão do repo: `testing` + `httptest`, sem bibliotecas novas.

`internal/devicelog`:

- rotação com limite pequeno: no máximo N arquivos, ordem `.1…` correta,
  mais velho apagado, cabeçalho em cada arquivo;
- desligado não cria arquivo; desligar apaga todos;
- sanitização: base64 → `<… KB>`, `password` → `***`, truncamento 2KB;
- campos-chave de JSON, XML, form e query;
- concorrência com `-race`: várias goroutines, nenhuma linha intercalada;
- middleware: handler recebe corpo intacto; linha tem origem, status,
  campos e corpo; desligado não lê nem grava.

`internal/handlers`:

- `/log`: `text/plain` inline, 404 com log desligado;
- `/log.zip`: zip válido com os arquivos esperados, 404 sem arquivos.

Emuladores:

- Hikvision e Dahua: requisição real ao servidor do emulador gera `IN`,
  `CONN aberta` e `CONN fechada`;
- push para um `httptest.Server` gera `OUT`.

Verificação manual:

- ligar o log com o emulador rodando;
- simular dois gerenciadores com `curl` de origens/portas diferentes e
  ver os dois endpoints no log;
- abrir pelo **Ver log**, baixar o zip;
- desligar e conferir que os arquivos sumiram.

## Fora de escopo

- Resumo semântico por handler ("cadastrou usuário Ana").
- Página visualizadora com filtro/cores.
- Retenção configurável pela UI.
- Autenticação no console.
