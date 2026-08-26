# Emuladores independentes do Invenzi — CRUD próprio

Data: 2026-08-26
Status: aprovado para planejamento

## Problema

Hoje o serviço só sabe existir emuladores que o W-Access mandou. A única
fonte de dispositivos é `WxsDB.GetLocalControllers()`, que varre
`CfgHWLocalControllers` procurando `LocalControllerDescription LIKE
'emulator%'`. Sem WXS configurado o serviço sobe, mas a lista de
dispositivos fica vazia para sempre — não há caminho nenhum, nem por UI
nem por API, para criar um emulador.

Pior: `cleanupOrphanedDevices()` apaga de `service.devices` tudo que não
apareceu na última resposta do WXS. Mesmo que alguém inserisse uma linha
à mão no banco, ela sumiria no refresh seguinte.

O objetivo desta versão é inverter a relação: o serviço passa a ser dono
dos seus emuladores, e o vínculo com o Invenzi vira uma opção. Ligado,
funciona como hoje. Desligado, o operador cadastra emuladores pela UI ou
por uma API REST de CRUD — inclusive em lote, informando porta inicial e
final — e pode apontar qualquer outro sistema de controle de acesso para
eles.

## Decisões

Tomadas com o usuário antes do design, registradas aqui porque cada uma
descarta alternativas que voltariam a aparecer durante a implementação:

| Decisão | Escolha | Alternativa descartada |
|---|---|---|
| Convivência WXS × manual | Coexistência: coluna `source`, sync com toggle | Modo exclusivo (trocar de modo destruiria o outro lado) |
| Identidade dos manuais | `local_controller_id` continua PK; manuais vêm de sequence começando em 900000 | PK genérica + `external_id` (migração de FK em 8 tabelas); IDs negativos |
| Superfície da API | `/api/emulators` novo, `/api/devices` intacto | Árvore `/api/v1` unificada; estender `/api/devices` |
| Autenticação | Nenhuma, como o resto do serviço | API key opcional ou obrigatória |
| Lote (range) | Só a porta varia; IP e prefixo de nome fixos | IP incrementando junto; template com placeholders |
| Porta em conflito | Rejeita o lote inteiro, nada é gravado | Pular e relatar; falhar só no start |
| Remoção | Purga os dados do emulador em transação | Manter órfãos; soft delete |
| Auto-start | Campo opcional, default parado | Sempre iniciar; nunca iniciar |

## Restrições do código atual

Três fatos do repositório que a implementação não pode ignorar.

**Não existe migration runner.** `ValidateDatabaseOnStartup()`
(`internal/database/validator.go`) confere se cada tabela e cada coluna
crítica existe; se faltar qualquer uma, executa `DROP SCHEMA emulator,
service CASCADE` e recria tudo a partir de
`assets/migrations/V001_create_emulator_schema.sql`, embutido no binário.
Isso torna o validator uma arma apontada para o banco do cliente:
**adicionar `source` à lista `criticalColumns` do validator apagaria os
dados de toda instalação existente no primeiro boot da versão nova.** A
lista do validator não pode ser tocada.

**O bind é `0.0.0.0`.** Tanto `hikvision/emulator.go` quanto
`dahua/emulator.go` escutam em `0.0.0.0:{porta}`. A coluna `ip_address`
nunca chega a um `net.Listen` — ela só é devolvida dentro das respostas
ISAPI e CGI, como metadado que o sistema testado lê. Um "range" é
portanto N portas no mesmo host, e faz sentido o IP ser fixo no lote.

**pgx/v4, e `device_settings` é chave/valor.** O módulo usa
`github.com/jackc/pgx/v4` e `github.com/jackc/pgconn` puro, não v5.
`emulator.device_settings` é `(device_id, cfg_id, value)` — o modo
online/standalone mora na chave `LocalAuthentication`, com `'1'` para
standalone e `'0'` para online.

## 1. Modelo de dados

Migração nova, `assets/migrations/V002_manual_emulators.sql`, com DDL
inteiramente idempotente:

```sql
ALTER TABLE service.devices
  ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'wxs';

ALTER TABLE service.devices
  DROP CONSTRAINT IF EXISTS devices_source_check;
ALTER TABLE service.devices
  ADD CONSTRAINT devices_source_check CHECK (source IN ('wxs','manual'));

CREATE SEQUENCE IF NOT EXISTS service.manual_device_id_seq START 900000;

ALTER TABLE service.wxs_settings
  ADD COLUMN IF NOT EXISTS sync_enabled BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS idx_devices_source ON service.devices(source);
```

O `DEFAULT 'wxs'` é o backfill: toda linha existente vira `source='wxs'`
sem nenhum passo extra, e uma instalação que hoje sincroniza com o
W-Access continua se comportando exatamente igual depois da atualização.

`sync_enabled` mora em `service.wxs_settings` porque a tela `/settings` já
lê e grava essa linha. Quando não há linha nenhuma — instalação nova, WXS
nunca configurado — o sync é considerado desligado, que já é o estado de
fato hoje.

### Runner de migração

Como não existe, precisa ser criado, pequeno e explícito:

```sql
CREATE TABLE IF NOT EXISTS service.schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

Contrato:

- `V001` continua sendo o baseline, de propriedade do validator. O runner
  não a executa e não a versiona; ao rodar pela primeira vez sobre um
  banco que já tem `service.devices`, registra `V001` como aplicada.
- O runner roda em `main.go` logo **depois** de
  `ValidateDatabaseOnStartup()` e **antes** de `GetServiceDB()`, aplicando
  em ordem lexicográfica toda migração `V0NN_*.sql` embutida ainda não
  registrada, cada uma dentro de uma transação.
- `assets/assets.go` ganha `MigrationFiles()` devolvendo o `fs.FS` de
  `migrations/`, mantendo `MigrationSQL()` como está para o validator.
- Falha em qualquer migração aborta a subida com `log.Fatalf`. Um banco
  meio migrado é pior que um serviço que não sobe.

Os dois caminhos convergem no mesmo esquema: instalação nova recebe o
baseline do validator e em seguida a V002; instalação existente passa na
validação e recebe só a V002.

## 2. Manager

### O sync deixa de ser incondicional

`cleanupOrphanedDevices()` ganha `AND source = 'wxs'` no `DELETE` e passa
a considerar órfãos apenas dispositivos dessa origem. Sem essa mudança
nada mais do desenho funciona: o primeiro refresh apagaria todo emulador
manual.

`RefreshDevices()` passa a checar o toggle antes de qualquer coisa e
devolve um erro tipado, `ErrSyncDisabled`, quando `wxsDB == nil` ou
`sync_enabled = false`. Erro tipado, e não `fmt.Errorf`, porque o handler
precisa distinguir "sync desligado" (409, estado esperado) de "WXS fora do
ar" (502) — hoje os dois virariam a mesma string.

O watchdog não muda: ele monitora emulador rodando, e emulador manual
roda igual.

### Métodos novos

```go
func (m *Manager) CreateDevice(spec DeviceSpec) (models.Device, error)
func (m *Manager) CreateDeviceRange(spec RangeSpec) ([]models.Device, error)
func (m *Manager) UpdateDevice(id int, spec DeviceSpec) (models.Device, error)
func (m *Manager) DeleteDevice(id int) error
```

Regras que valem para os quatro:

- **Só operam em `source='manual'`.** `UpdateDevice` e `DeleteDevice`
  sobre um dispositivo vindo do WXS retornam `ErrDeviceIsManaged` → 409.
  A verdade dele mora no W-Access; o próximo sync sobrescreveria a edição
  em silêncio, e é melhor recusar do que fingir que funcionou.
- **`start`, `stop`, `settings` e `mode` continuam valendo para os dois.**
  São operações de execução, não de cadastro, e nada no WXS as
  sobrescreve.
- **Edição exige o emulador parado.** O emulador guarda uma cópia de
  `models.Device` em memória, criada no `createEmulator()`; alterar nome,
  IP ou porta com ele rodando deixaria as respostas ISAPI/CGI mentindo, e
  a porta em uso divergindo da cadastrada. Rodando → `ErrDeviceRunning` →
  409 com "pare o emulador antes de editar".
- **`DeleteDevice` para o emulador antes de purgar**, sem exigir que o
  operador pare na mão: remover é uma intenção inequívoca, editar não.

### Alocação de ID

`nextval('service.manual_device_id_seq')` dentro da mesma transação da
inserção. A sequence começa em 900000, acima de qualquer
`LocalControllerID` plausível de um W-Access real. Se um dia colidir, o
`INSERT` viola a PK e a transação inteira volta atrás — falha barulhenta,
não corrupção silenciosa.

## 3. API

```
GET    /api/emulators          lista (payload de /api/devices + source)
POST   /api/emulators          cria um
POST   /api/emulators/range    cria em lote
PUT    /api/emulators/:id      edita
DELETE /api/emulators/:id      remove e purga
```

`/api/devices` fica intacto — nada que já consome quebra, e a UI atual
segue funcionando durante toda a implementação.

### Criar um

```jsonc
POST /api/emulators
{
  "name": "lab-01",
  "model": "Hikvision",
  "ip_address": "192.168.1.50",
  "port": 4001,
  "event_interval": 10,
  "enabled": true,
  "auto_start": false
}
// 201 → { "id": 900001, "name": "lab-01", "port": 4001, "status": "stopped", ... }
```

`event_interval` default 10, `enabled` default `true`, `auto_start`
default `false`, `ip_address` default `"127.0.0.1"`.

### Criar em lote

```jsonc
POST /api/emulators/range
{
  "name_prefix": "lab",
  "model": "Dahua",
  "ip_address": "192.168.1.50",
  "port_start": 4000,
  "port_end": 4049,
  "event_interval": 10,
  "enabled": true,
  "auto_start": false
}
// 201 → { "count": 50, "created": [ { "id": 900001, "name": "lab-4000", "port": 4000 }, ... ] }
```

Nome de cada item: `{name_prefix}-{porta}`. A porta no nome, e não um
índice sequencial, porque a porta é o que o operador procura quando algo
falha.

### Editar

`PUT /api/emulators/:id` recebe o mesmo corpo do `POST` de item único,
menos `auto_start`, e é **substituição total** dos campos editáveis:
`name`, `model`, `ip_address`, `port`, `event_interval`, `enabled`.
Campo ausente cai no default, não mantém o valor anterior — a UI envia o
formulário inteiro, e um PATCH parcial seria uma segunda semântica para
pouca coisa.

### Validação

Igual para os dois verbos de criação e para o `PUT`:

- `model` ∈ `{"Hikvision", "Dahua"}` — são os únicos que
  `createEmulator()` sabe instanciar.
- `1 ≤ port ≤ 65535`; no lote, `port_end ≥ port_start`.
- Lote de no máximo 1000 portas, que é a largura do range publicado no
  `docker-compose.yml` (4000-4999).
- `name` / `name_prefix` não vazio.
- **Colisão de porta contra todos os dispositivos cadastrados**, WXS
  inclusive, e contra as portas do próprio lote. Qualquer conflito →
  `400` com a lista completa, e **nada é gravado**:

```jsonc
{ "error": "portas em conflito", "conflicts": [4003, 4004] }
```

A checagem e a inserção acontecem na mesma transação, serializadas por
`pg_advisory_xact_lock` com uma chave fixa do módulo — duas criações
simultâneas não podem passar pela validação e colidir na gravação. Um
`UNIQUE` em `service.devices(port)` seria mais direto, mas quebraria a
migração de qualquer instalação cujo W-Access já tenha portas repetidas,
e o esquema atual não impede isso.

Colisão contra a porta HTTP do próprio serviço (`cfg.Server.Port`, 8080
por padrão) também é conflito, e vale a pena o erro dizer isso com todas
as letras.

### Códigos de resposta

| Situação | Código |
|---|---|
| Criado | 201 |
| Editado / removido | 200 |
| Payload inválido, porta em conflito | 400 |
| ID inexistente | 404 |
| Dispositivo é do WXS; ou está rodando e a edição exige parado | 409 |
| Falha de banco | 500 |

## 4. Remoção purga os dados

Não há FK entre `service.devices` e as tabelas `emulator.*` — todas
carregam `device_id` solto. A limpeza é explícita, numa transação, na
ordem:

```
emulator.dahua_cards, emulator.dahua_faces,
emulator.hikvision_users, emulator.hikvision_cards,
emulator.hikvision_faces, emulator.hikvision_fingers,
emulator.device_settings, service.users_comparison,
service.devices
```

`emulator.device_settings` tem `device_id = 0` para os padrões globais
semeados na V001; a purga filtra pelo `device_id` do dispositivo, então
essa linha nunca é tocada.

## 5. Interface

**`devices.html`** ganha:

- botões **Novo emulador** e **Criar em lote**, abrindo modais que batem
  em `POST /api/emulators` e `POST /api/emulators/range`;
- badge de origem por linha (WXS / Manual);
- ações **Editar** e **Remover** visíveis apenas em linhas manuais, com
  confirmação na remoção que diz explicitamente que os cadastros de
  cartões e faces daquele emulador vão junto.

**`settings.html`** ganha, no bloco WXS, o toggle **Sincronizar com
Invenzi W-Access**. Desligado, o botão "Atualizar do WXS" fica
desabilitado, com o motivo visível em vez de um clique que devolve erro.

O JS novo segue o padrão de `devices.js` e `settings.js`, com feedback
via `toast.js`.

## 6. Testes

TDD, testes antes da implementação. Cobertura mínima:

`internal/emulator` (manager):
- cria um; cria lote; ID sai da sequence acima de 900000
- lote com colisão não grava nada — a contagem antes e depois é igual
- lote colidindo internamente (mesma porta duas vezes) é rejeitado
- `DeleteDevice` purga as sete tabelas e não toca em `device_id = 0`
- `cleanupOrphanedDevices` preserva `source='manual'` e remove `'wxs'`
- `RefreshDevices` com sync desligado devolve `ErrSyncDisabled`
- editar dispositivo do WXS devolve `ErrDeviceIsManaged`
- editar dispositivo rodando devolve `ErrDeviceRunning`

`internal/handlers`: contratos HTTP no padrão `*_http_test.go` já
existente — códigos de status, corpo do erro de conflito, `Content-Type`.

`internal/database`: runner aplica V002 uma vez, é idempotente ao rodar
duas vezes, e registra `V001` como baseline sobre banco preexistente.

## 7. Documentação

`PORTAS-EMULADORES.md` afirma hoje, em negrito, que "as portas dos
emuladores **NÃO são definidas pela aplicação Go**. Elas vêm do sistema
WXS". Deixa de ser verdade e precisa ser reescrito: a seção passa a
descrever as duas origens, e o range 4000-4999 do compose passa a ser o
limite recomendado do cadastro em lote.

`README.md` ganha a seção da API de CRUD com exemplos de `curl`.

## Fora de escopo

- Autenticação, na API ou na UI.
- Modelos além de Hikvision e Dahua.
- Importar um dispositivo do WXS convertendo-o em manual.
- Editar dispositivos do WXS pelo serviço.
- Migrar a PK para um identificador genérico com `external_id`.
