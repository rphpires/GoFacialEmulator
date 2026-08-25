# Hikvision Message Fidelity — Audit & Fix Design

**Data:** 2026-04-17
**Escopo:** Auditoria + plano de correção das mensagens emitidas pelo emulador Hikvision para alinhá-las ao comportamento observado em um equipamento DS-K1T673DX-BR real.
**Motivação:** O cliente Python (`IoHikvisionCommunication`) está logando `xml.parsers.expat.ExpatError: no element found: line 1, column 0` em `__get_events`. Investigação revelou múltiplas divergências entre o que o emulador envia e o que o dispositivo real envia; o erro reportado é apenas o sintoma mais visível.

## 1. Material de referência

Captura Wireshark do dispositivo real (IP 192.168.1.81, modelo DS-K1T673DX-BR, firmware V3.18.0) salva em [aux_files/Emulator hikvision test/](../../../aux_files/Emulator%20hikvision%20test/). Streams TCP relevantes extraídos em texto:

- `stream_49_capabilities.txt` — `GET /ISAPI/AccessControl/capabilities?format=json`
- `stream_57_system_time.txt` — `PUT /ISAPI/System/time`
- `stream_60_httphosts_put.txt` — `PUT /ISAPI/Event/notification/httpHosts`
- `stream_62_deviceinfo.txt` — `GET /ISAPI/System/deviceInfo`
- `stream_67_storagecfg.txt` — `PUT /ISAPI/AccessControl/AcsEvent/StorageCfg?format=json`
- `stream_69_remotecontrol_door.txt` — `PUT /ISAPI/AccessControl/RemoteControl/door/1`
- `stream_75_acscfg.txt` — `PUT /ISAPI/AccessControl/AcsCfg?format=json`
- `stream_77_alertstream.txt` — `GET /ISAPI/Event/notification/alertStream` (long-poll com vários eventos)
- `stream_80_waccess_post.txt` — `POST /w-access` (push do dispositivo para o cliente)
- `stream_104_remoteCheck.txt` — `PUT /ISAPI/AccessControl/remoteCheck?format=json`

Esses dumps são a **verdade de referência** para todo golden test e validação manual descritos abaixo.

## 2. Decisões de escopo (acordadas no brainstorming)

- **Fidelidade funcional** (não fidelidade máxima): replicamos só o que o cliente realmente consome. Pulamos digest auth (cliente tolera ausência) e headers cosméticos (`Server`, `X-Frame-Options`, `X-XSS-Protection`).
- **Variedade de eventos não muda**: o emulador continua gerando apenas o evento `majorEventType:5, subEventType:75` (card+face). Outros tipos observados no real (`3/1025`, `3/1028`, `5/21`, `5/22`, `3/121`) ficam para uma rodada futura.
- **Endpoints internos de management** (`UserInfo/*`, `CardInfo/*`, `FDLib/*`, `FingerPrint*`) **não são auditados** porque não aparecem no tráfego do dispositivo real — são extensões da aplicação para registrar usuários no emulador.

## 3. Inventário de auditoria

### 3.1. Endpoints servidos pelo emulador (cliente → emulador)

| Endpoint | Real device | Emulador hoje | Divergência | Fase |
|---|---|---|---|---|
| `GET /ISAPI/AccessControl/capabilities?format=json` | header `application/json`, body XML (quirk!) | header `application/xml`, body XML | Content-Type incorreto quando `?format=json` | F3 |
| `PUT /ISAPI/AccessControl/Door/param/1` | XML in, ResponseStatus XML out | XML out (struct) | ok | — |
| `GET /ISAPI/System/time` | `<Time>` XML | `c.XML(string)` com `<DeviceInfo>` | **GRAVE** — forma errada + dupla codificação Gin | F1 |
| `PUT /ISAPI/System/time` | XML in, ResponseStatus XML out | retorna `"OK"` plain text | Errado | F1 |
| `PUT /ISAPI/AccessControl/AcsEvent/StorageCfg?format=json` | JSON in/out | JSON ResponseStatus | ok | — |
| `GET /ISAPI/Event/notification/httpHosts` | XML, ecoa `parameterFormatType` salvo | XML, hardcoda `parameterFormatType=XML` | Não persiste valor do PUT | F3 |
| `PUT /ISAPI/Event/notification/httpHosts` | XML in (cliente envia `parameterFormatType=JSON`), ResponseStatus XML out | XML in/out, mas descarta `parameterFormatType` | Não persiste valor | F3 |
| `PUT /ISAPI/AccessControl/AcsCfg?format=json` | JSON in/out | JSON in/out | ok (corrigido em commit 8b2dd83) | — |
| `GET /ISAPI/System/deviceInfo` | XML | XML (raw string) | ok | — |
| `PUT /ISAPI/AccessControl/RemoteControl/door/:n` | XML in/out | `c.XML(struct)` | ok | — |
| `GET /ISAPI/Event/notification/alertStream` | `multipart/mixed; boundary=MIME_boundary`, sem heartbeat inicial | `multipart/x-mixed-replace`, heartbeat inicial imediato | Content-Type + heartbeat extra | F1 |
| `PUT /ISAPI/AccessControl/remoteCheck?format=json` | JSON in/out | **não existe** (404) | Endpoint faltando | F2 |

### 3.2. Mensagens enviadas pelo emulador (emulador → cliente)

| Caminho | Real device | Emulador hoje | Divergência | Fase |
|---|---|---|---|---|
| Push event (POST `/w-access` ou URL configurada via httpHosts) | multipart com part `Content-Disposition: form-data; name="event_log"` + JSON | parte JSON SEM `Content-Disposition` | **GRAVE** — quebra parser do cliente Python | F1 |
| Push event com face | + part `Content-Disposition: form-data; name="Picture"` + JPEG | já correto | ok | — |
| Stream event no alertStream | parte JSON SEM `Content-Disposition` (porque é `multipart/mixed`, não form-data) | tem `name="event_log"` (sobrando) | inverso: parser tolera, mas é drift | F4 |

### 3.3. Payload JSON do evento (`Event` struct)

| Campo | Real | Emulador | Fase |
|---|---|---|---|
| `shortSerialNumber` | `"AA8066966"` | ausente | F4 |
| `turnstileTurned` (em card events) | `false` | ausente | F4 |
| `licensePlateNo` | `""` | ausente | F4 |
| `mask` em non-card events | `"unknown"` | n/a (só geramos card events) | n/a |
| Indentação JSON | indented com 1 espaço | `MarshalIndent(..., "  ")` (2 espaços) | F4 (cosmético) |

### 3.4. Outros bugs identificados no código

| Local | Problema | Fase |
|---|---|---|
| `handleSetDateTime` (handlers.go:933) | Retorna `"OK"` plain text em vez de ResponseStatus XML | F1 |
| `handleCommandOutput` (handlers.go:970) | Retorna `"OK"` plain text | F1 (baixa prioridade) |
| `handlePostFingerprintSetup` (handlers.go:671) | Retorna `"OK"` plain text | F1 (baixa prioridade) |
| `handlePutFingerprintDelete` (handlers.go:794) | Retorna `"OK"` plain text | F1 (baixa prioridade) |

### 3.5. Resumo de divergências por gravidade

- **Bloqueantes** (provavelmente causam o erro `__get_events`): Content-Disposition faltante no push, dupla codificação no `handleGetDateTime`, retorno plain text de `handleSetDateTime`, Content-Type errado do alertStream.
- **Funcionais** (cliente faz request e recebe 404 ou comportamento errado): `remoteCheck` faltando, `parameterFormatType` não persistido.
- **Cosméticos** (não quebram o cliente, mas afetam fidelidade): campos do JSON do evento, headers extras na resposta, indentação.

## 4. Plano por fases

Quatro PRs independentes. Cada fase pode ser pausada sem deixar o emulador em estado inconsistente. F1 sozinha já mata o erro reportado.

### 4.1. F1 — Bug fixes isolados

**Objetivo:** parar o erro `__get_events` reportado e corrigir respostas que claramente quebram o contrato.

| # | Mudança | Arquivo:linha | Como |
|---|---|---|---|
| 1 | Adicionar `Content-Disposition: form-data; name="event_log"` na parte JSON do `sendEventToRemoteServer` | `internal/emulator/hikvision/emulator.go:336-341` | Inserir o header antes do `Content-Type` na primeira parte do multipart |
| 2 | Reescrever `handleGetDateTime` para retornar `<Time>` correto via `c.String()` | `internal/emulator/hikvision/handlers.go:925-931` | Body: `<?xml version="1.0" encoding="UTF-8"?>\n<Time version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">\n<timeMode>manual</timeMode>\n<localTime>{nowISO}</localTime>\n<timeZone>CST+3:00:00</timeZone>\n</Time>`. Header `application/xml`. `nowISO` = `time.Now().Format("2006-01-02T15:04:05-07:00")` |
| 3 | Reescrever `handleSetDateTime` para retornar ResponseStatus XML | `internal/emulator/hikvision/handlers.go:933-935` | Substituir `c.String(200, "OK")` por `writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")` (helper já existe em handlers.go:1063) |
| 4 | Trocar `multipart/x-mixed-replace` por `multipart/mixed` no alertStream | `internal/emulator/hikvision/handlers.go:1106` | Mudar a string do `Content-Type` na composição do `httpResponse` |
| 5 | Remover heartbeat inicial imediato no alertStream | `internal/emulator/hikvision/emulator.go:597-608` | Apagar bloco que chama `e.getHeartbeatMessage()` antes do loop, mantendo o heartbeat periódico do loop principal |
| 6 | Remover header `Cache-Control: no-cache` da resposta do alertStream | `internal/emulator/hikvision/handlers.go:1108` | Remover linha (real não envia) |
| 7 | Trocar `c.String("OK")` por `writeHikvisionXML(...)` em `handleCommandOutput`, `handlePostFingerprintSetup`, `handlePutFingerprintDelete` | handlers.go:670-674, 794-817, 970-974 | Substituições simples |

**Critério de pronto:** o cliente Python para de logar `xml.parsers.expat.ExpatError: no element found` e a tela de "polling form socket" começa a processar eventos.

### 4.2. F2 — Endpoints faltantes

**Objetivo:** parar de retornar 404 nos endpoints que o cliente chama em todo evento.

| # | Endpoint | Comportamento |
|---|---|---|
| 1 | `PUT /ISAPI/AccessControl/remoteCheck?format=json` | Aceita JSON `{"RemoteCheck": {...}}`, loga, responde JSON `{"statusCode":1,"statusString":"OK","subStatusCode":"ok"}`. O emulador não toma decisão de acesso — só sinaliza OK para destravar o fluxo do cliente |

**Sobre o payload do `remoteCheck`:** o stream 104 do pcap mostra o cabeçalho mas truncou o body em vários frames. Para a primeira implementação, o handler aceita qualquer JSON com chave `RemoteCheck` e responde OK. Se for necessário no futuro, expandimos para inspecionar o conteúdo.

**Critério de pronto:** logs do cliente não mostram mais erros HTTP 404 para `/remoteCheck`.

### 4.3. F3 — Negociação de formato e persistência de `parameterFormatType`

**Objetivo:** suportar o que o cliente real espera quando ele decide o formato dos eventos.

**Mudança 1 — Persistir `parameterFormatType` do PUT httpHosts.**
- Em `handlePutHttpHosts` (handlers.go:1017): adicionar o campo `ParameterFormatType string \`xml:"parameterFormatType"\`` no struct `HttpHostNotificationItem` (handlers.go:37). O campo `URL` já existe no struct. O parser `parseHttpHostNotification` (handlers.go:47) passa a popular automaticamente o novo campo via `xml.Unmarshal`.
- Persistir em settings com chave `RemoteParameterFormat` (default `"JSON"` se não vier no payload).
- No `handleGetHttpHosts` (handlers.go:978), ler o setting e injetar no template XML em vez do `parameterFormatType` hardcoded.

**Mudança 2 — Usar formato persistido ao enviar push events.**
- Em `sendEventToRemoteServer` (emulator.go:317): ler `RemoteParameterFormat`. Se `JSON` (caso real do user), manter o multipart atual. Se `XML`, marshal do evento para XML em vez de JSON.
- O branch XML pode ser implementado como stub que loga "XML push not implemented" e retorna sem enviar — documentado mas não exercitado, porque o cliente alvo usa JSON.

**Mudança 3 — Negociação `?format=json` no GET capabilities.**
- Em `handleGetAccessControlCapabilities` (handlers.go:195): se `c.Query("format") == "json"`, responder com header `Content-Type: application/json` mantendo o body XML (replicando o quirk do real, confirmado no stream 49). Caso contrário, header `application/xml`.

**Critério de pronto:** o GET httpHosts ecoa de volta exatamente o `parameterFormatType` que o cliente enviou no PUT; o capabilities responde com header correto conforme query param.

### 4.4. F4 — Alinhamento do JSON do evento

**Objetivo:** zerar drift entre o payload do evento que o emulador gera e o do dispositivo real.

| # | Mudança | Local |
|---|---|---|
| 1 | Adicionar `ShortSerialNumber string` no struct `Event` e popular em `generateOnlineEvent` e `generateRandomEvent` | `internal/emulator/hikvision/models.go:127` + `internal/emulator/hikvision/emulator.go:260, 473` |
| 2 | Adicionar `TurnstileTurned bool` em `AccessControllerEvent` | mesmo struct |
| 3 | Adicionar `LicensePlateNo string` em `AccessControllerEvent` | mesmo struct |
| 4 | Trocar `MarshalIndent(event, "", "  ")` por `MarshalIndent(event, "", " ")` (1 espaço) para bater com o real | `internal/emulator/hikvision/emulator.go:319, 518` |
| 5 | Remover `Content-Disposition: form-data; name="event_log"` da parte JSON do alertStream (real não tem — `multipart/mixed` não usa esse header) | `internal/emulator/hikvision/emulator.go:536` |

**Critério de pronto:** diff entre payload do emulador e captura real (`stream_77_alertstream.txt`) só mostra valores variáveis (timestamp, números aleatórios, `name`, `cardNo`, `employeeNoString`).

## 5. Estratégia de testes

Padrão dos testes segue o existente em [handlers_httphosts_test.go](../../../internal/emulator/hikvision/handlers_httphosts_test.go) (`httptest` + Gin engine).

### 5.1. Por fase

**F1 — Bug fixes**
- **Unit test** para cada handler corrigido, verificando: (a) header `Content-Type` exato, (b) body começando com `<?xml version="1.0"...` (ou ResponseStatus correto), (c) ausência de tags `<string>` (sintoma da dupla codificação que estamos corrigindo).
- **Golden file** para o multipart push: armar evento, capturar bytes do `body.String()` em `sendEventToRemoteServer`, comparar contra arquivo de referência derivado de `stream_80_waccess_post.txt` (mascarando bytes variáveis: timestamps, randoms).

**F2 — `remoteCheck`**
- Unit test: POST JSON arbitrário com chave `RemoteCheck`, esperar 200 + body JSON `{"statusCode":1,...}`. Garante apenas que o handler existe e responde no formato correto.

**F3 — Format negotiation e persistência**
- Unit test do parser estendido `parseHttpHostNotification`: feed XML real do `stream_60_httphosts_put.txt`, verificar que extrai `ParameterFormatType="JSON"`, `URL="/w-access"`, `IPAddress="192.168.1.138"`, `PortNo="15501"`.
- Round-trip test: PUT com XML real → GET → comparar `parameterFormatType` no body resposta = valor enviado.
- Test do `?format=json` no capabilities: dois requests, verificar que header `Content-Type` muda (`application/xml` vs `application/json`) mas o body é o mesmo XML.

**F4 — Payload alignment**
- Test de schema do `Event`: marshal um evento populado, descodificar para `map[string]any`, verificar presença das chaves `shortSerialNumber`, `turnstileTurned`, `licensePlateNo`.
- (Opcional) golden file: diff do JSON do evento contra um payload extraído do `stream_77_alertstream.txt`, ignorando campos variáveis.

### 5.2. Validação end-to-end (manual)

Depois de cada fase mergeada, rodar o emulador contra o cliente Python real do usuário e checar o log:
- F1: para de aparecer `xml.parsers.expat.ExpatError`.
- F2: para de aparecer `404 /remoteCheck`.
- F3: log do cliente mostra `parameterFormatType: 'JSON'` no `web_server_info_in_device` (em vez de `'XML'`).
- F4: comparação visual de um evento logado pelo cliente vs uma captura real.

### 5.3. O que não é testado

- Digest auth — fora de escopo.
- Endpoints de management (`UserInfo/Search`, `CardInfo/*`, `FDLib/*`) — não tocados.
- Performance/load — fora de escopo.

## 6. Riscos e mitigações

| Risco | Mitigação |
|---|---|
| Mudar `multipart/x-mixed-replace` para `multipart/mixed` quebra o pull-mode em algum cliente que dependia do `x-mixed-replace` (semântica de "substituir") | O cliente do user é o alvo prioritário; ele consome `multipart/mixed` no real. Se algum outro consumidor existir, é um caso futuro. |
| Remover heartbeat inicial faz o cliente travar esperando bytes | Real não envia inicial e funciona. Se travar, voltamos a um comportamento intermediário (esperar um pouco antes de heartbeat). |
| `RemoteParameterFormat` setado para XML ativa branch não implementado de marshal XML | Stub loga claramente; cliente real do user usa JSON, então não exercita. |
| `MarshalIndent` com 1 espaço pode mudar semanticamente ordem de chaves no JSON | Não muda — Go preserva ordem do struct. Só muda whitespace. |

## 7. Próximos passos após aprovação

1. Commit do spec.
2. Invocar `superpowers:writing-plans` para gerar o plano de execução detalhado da **F1** (apenas — F2/F3/F4 ganham seus próprios planos quando chegar a hora).
3. Executar F1, validar contra o cliente Python real, abrir PR.
