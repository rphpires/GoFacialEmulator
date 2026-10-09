# Checklist de validação — requisitos do Emulador

Itens a validar na aplicação atual do Emulador. Marcar `[x]` quando validado e
anotar evidência/observação ao lado (versão, data, link de teste ou lacuna encontrada).

Registrado em 2026-10-09. Levantamento do código em 2026-10-09
(branch `feat/compat-endpoints-gerenciador`, commit `edae88e`).

Legenda: **✅ atendido** · **🛠 a implementar** (ver fase na
[sequência de implantação](#sequência-de-implantação)) · **⏸ adiado** · **🚫 fora de escopo**.

**Escopo:** foco no Emulador. Pontos de gestão de usuários/pessoas no W-Access
(seção 4 e validação de sincronização de pessoas) ficam fora.

## 1. Instalação e portabilidade

- [x] ✅ Instalação simples em qualquer máquina, sem passos manuais extensos.
  - Pacotes ZIP `docker|windows|linux` (`packaging/build-pacotes.{sh,bat}`); fluxo INSTALAR → INICIAR → `http://localhost:7070`. Windows embute PostgreSQL portátil.
- [x] ✅ Opções de instalação: executável/serviço Windows, container Docker ou modo portátil.
  - Suportados: Docker, executável Windows + Postgres portátil (modo portátil), Linux/WSL2. Serviço Windows (nssm/winsw) ⏸ adiado — só se houver demanda de rodar sem sessão logada.
- [x] ✅ Pré-requisitos e portas utilizadas documentados.
  - `docs/manual/conteudo/{windows,linux,docker}/antes-de-comecar.md`, `portas-e-rede.md`, `PORTAS-EMULADORES.md`, `README.md`, LEIA-ME dos pacotes.
- [ ] 🛠 **F5** Exportação e importação de configuração, para replicar o mesmo cenário em outra máquina.
  - Hoje só manual (copiar `configs/config.yaml` + dump do Postgres).

## 2. Cadastro dos emuladores no W-Access (substitui o script atual)

> Hoje: `scripts/bulk-insert-emulators.sql`, rodado à mão no SSMS. A aplicação só **lê** o W-Access.
> Cadastra **controladores** (não usuários), por isso fica no escopo, mas é a fase que escreve no
> banco do W-Access — ver **F8**.

- [ ] 🛠 **F8** Função disponível dentro da interface dos emuladores, não mais em script separado.
- [ ] 🛠 **F8** Escolha da localidade onde os controladores serão criados.
- [ ] 🛠 **F8** Definição da quantidade de controladores por localidade.
- [ ] 🛠 **F8** Escolha do gerenciador (servidor/serviço) para o qual cada controlador aponta.
- [ ] 🛠 **F8** Escolha do fabricante emulado (Dahua, Hikvision ou ambos).
- [ ] 🛠 **F8** Remoção ou limpeza dos controladores criados, para resetar o ambiente de teste.

## 3. Simulação de acessos

- [x] ✅ Botão para ligar e desligar a simulação de acessos, por emulador, por grupo ou para todos.
  - Start/Stop por emulador, por seleção múltipla (seleção após filtro = grupo) e "todos".
- [ ] 🛠 **F2** Opção de simular acesso com foto ou sem foto.
  - Hoje todo evento leva a mesma foto fixa.
- [x] ✅ Taxa de eventos configurável (ex.: acessos por minuto por controlador).
  - `event_interval` por controlador (1 evento a cada N s). Ajuste a quente fica para **F7** (rampa).
- [ ] 🛠 **F1** Mistura configurável de eventos: acesso concedido, negado e usuário desconhecido.
  - Hoje: Hikvision standalone com pesos fixos no código; Hikvision online sempre 75; Dahua sempre concedido; "face não identificada" (333) nunca é gerada de forma reconhecível pelo gerenciador.

## 4. Geração de carga adicional

- 🚫 Atualização de pessoas em massa (inclusão/alteração/exclusão, com e sem foto) — gestão de usuários no W-Access, fora de escopo. Continua via `scripts/bulk-insert-users.sql` e `scripts/simulate-operations.py`.
- 🚫 Volume e frequência da carga de pessoas — idem.

## 5. Monitoramento e resultados

- [ ] 🛠 **F3** Painel com o status de cada emulador (online/offline, conectado ao gerenciador).
  - Já existe Estado (running/stopped) ao vivo via SSE. Falta "conectado ao gerenciador".
- [ ] 🛠 **F3** Contadores de eventos enviados e de eventos confirmados pelo W-Access.
  - "Confirmado" = gerenciador respondeu 2xx ao push (Hikvision `httpHosts`, Dahua) ou consumiu o evento no stream.
- [x] ✅ Log de erros e timeouts.
  - `logs/trace.log` + `trace.html` com rotação, log por dispositivo. Contagem de erros/timeouts por emulador entra junto com **F3**.
- [ ] 🛠 **F4** Exportação dos resultados do teste (CSV ou relatório).

## Sugestões adicionais

- [ ] 🛠 **F7** Cenários salvos (perfis de teste) — reaproveita export/import de **F5**.
- [ ] 🛠 **F6** Simulação de falhas: derrubar conexão, reboot, perda de rede; reconexão e buffer offline.
  - Já existe histórico circular consultável pós-offline (Hikvision `AcsEvent`, Dahua `AccessControlCardRec`) e watchdog.
- [ ] 🛠 **F7** Rampa de carga.
- [ ] 🛠 **F3** (parcial) Métricas de latência — só o lado do emulador: tempo de resposta do push ao gerenciador. Latência até o registro no W-Access 🚫 fora de escopo (exige ler o banco do W-Access).
- 🚫 Validação de sincronização de pessoas emulador × W-Access — gestão de usuários.
  - Obs.: `/comparison` tem bug (`site_controller_count` sempre 0, `internal/handlers/handlers.go:848`; regra OK em `:787`). Corrigir só se a página continuar em uso.

---

## Sequência de implantação

Ordem pensada por valor para o teste de carga × dependência. F1–F4 formam o núcleo
(simular realista + medir); F5–F7 tornam os testes repetíveis; F8 é a frente que escreve no W-Access.

### F1 — Mistura de eventos configurável (inclui 333 "Face não identificada")

Referência do gerenciador (`wxs-site-controller`): `Events.BIOMETRIC_VERIFICATION_FAILED = 333`
(`GlobalConstants.py:429`). Condições que geram 333 / negado:

| Resultado no W-Access | Hikvision (o que o emulador deve enviar) | Dahua (o que o emulador deve enviar) |
|---|---|---|
| Concedido (`TRANSIT_EFFECTED`) | `subEventType` 75/1/38 **com** cartão | `ErrorCode=0`, `Status=1` |
| **333 Face não identificada** | `subEventType=76` (ou 39) **sem** `cardNo`/`employeeNo` (`IoHikvisionCommunication.py:1281`; online `:3053`) | `ErrorCode=16` **sem** `CardNo` (`IoDahuaCommunication.py:2490`) ou `Type=Face` sem cartão (`:1102`); offline `Status≠1` sem cartão (`:677`) |
| Cartão inválido (`INVALID_CARD`) | `subEventType=9` **com** cartão | `ErrorCode=16` **com** `CardNo` (`:2478`) |
| Cartão expirado (`CARD_EXPIRED`) | `subEventType=8` com cartão | — |

Entregas:
1. Config por emulador: pesos `concedido / negado / desconhecido` (padrão = comportamento atual, ex. 100/0/0 Dahua, 80/20/0 Hikvision). Persistir em `service.devices`; editar no formulário e em lote.
2. Hikvision: tirar `pesoSubEvento` fixo de `hikvision/subevents.go`, sortear categoria pelo peso; "desconhecido" → 76 sem cartão/`employeeNo`. Modo online: desconhecido não passa por `remoteCheck` (o equipamento real recusa localmente).
3. Dahua: gerar negado (`ErrorCode=16` com cartão) e desconhecido (`ErrorCode=16`/`Type=Face` sem `CardNo`, `Status=0`) no push online e no histórico `AccessControlCardRec`.
4. Testes unitários do sorteio + teste de payload por fabricante validando os campos da tabela acima.

### F2 — Acesso com/sem foto

1. Config por emulador: `% de eventos com foto` (0–100, padrão 100).
2. Hikvision: omitir a parte multipart da imagem / `pictureURL`; Dahua: omitir o anexo de imagem do evento.
3. "Desconhecido" (F1) com foto = cenário típico de face estranha; garantir que combina.

### F3 — Monitoramento por emulador

1. Contadores em memória por dispositivo: eventos gerados, enviados, confirmados (2xx/consumidos), falhas, timeouts; último envio OK.
2. "Conectado ao gerenciador": Dahua/Hikvision stream com cliente ativo, ou último push OK < N s.
3. Latência do push (média/p95) por emulador.
4. Expor em `/api/devices` + SSE; colunas no painel e detalhe na gaveta do dispositivo.

### F4 — Exportação de resultados

1. `GET /api/results.csv` (por emulador: contadores de F3, mix efetivo, período) e botão "Exportar" no painel.
2. Zerar contadores ao iniciar um teste ("Novo teste") para resultados comparáveis.

### F5 — Exportar/importar configuração

1. `GET /api/config/export` → JSON com emuladores manuais (portas, modelo, intervalo, mix, foto), configurações do console e conexão W-Access (senha opcional/omitida).
2. `POST /api/config/import` com prévia e modo substituir/mesclar; botões em Configurações.

### F6 — Simulação de falhas

1. Ações por emulador/seleção: "Derrubar conexão" (fecha stream/sockets e recusa novas conexões por N s), "Perda de rede" (para de responder, mas continua gerando eventos no buffer), "Reboot" (stop + start com atraso, zera histórico como equipamento real).
2. Fila de reenvio no push online (hoje falha só é logada) com limite, para testar recuperação.
3. Contadores de F3 refletem eventos gerados durante a queda × recuperados.

### F7 — Cenários salvos e rampa de carga

1. Perfil = intervalo + mix (F1) + foto (F2) + seleção de emuladores; salvar/aplicar; reusa formato de F5.
2. Ajuste a quente de intervalo/mix sem parar o emulador (pré-requisito da rampa).
3. Rampa: de X para Y eventos/min em N passos de T min; para automaticamente se taxa de falha/timeout de F3 passar de um limite.

### F8 — Cadastro de controladores no W-Access pela interface

Substitui `scripts/bulk-insert-emulators.sql`. Escreve no SQL Server do W-Access — exige
usuário com permissão de escrita e confirmação explícita na UI.

1. Listar localidades e gerenciadores (site controllers) do W-Access para os seletores.
2. Formulário: localidade, quantidade, gerenciador, fabricante (Dahua/Hikvision/ambos → tipos de `HIKVISION_CONTROLLER_TYPES`/`DAHUA_CONTROLLER_TYPES`), porta inicial, intervalo.
3. Mesma sequência do script em transação: `CfgHWLocalControllers` → `wsp_AddReader` → `CfgHWReaders` → `CfgACAccessLevelsContents`.
4. Marcar os criados (descrição `emulator_*` + marcador próprio) e oferecer "Remover controladores do emulador" só sobre os marcados.
