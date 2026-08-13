# Instalação simplificada — 3 pacotes ZIP + manual único

**Data:** 2026-08-13
**Status:** aprovado (aguardando plano de implementação)

## Problema

O usuário final do GoFacialEmulator é um técnico de campo com conhecimento técnico
limitado, de um único cliente interno. Hoje a instalação exige escolher entre cinco
pacotes de nomes opacos (`dist`, `dockerdb`, `embedded`, `portable`, `standalone`),
baixar binários do PostgreSQL manualmente e editar YAML. Três dos cinco pacotes estão
quebrados e a documentação aponta para arquivos que não existem.

### Defeitos verificados no repositório

1. **Build scripts desatualizados.** `build-standalone.bat`, `build-portable.bat` e
   `build-dockerdb.bat` copiam `web\static`, `web\templates` e
   `internal\database\migrations`. Essas pastas foram movidas para `assets/` e passaram
   a ser embutidas via `go:embed` no commit `13acc36`. Os três pacotes saem incompletos.
   Apenas `build-embedded.bat` está coerente com o código atual.

2. **Packaging e documentação fora do git.** `.gitignore` contém `dist-*/`,
   `build-*.bat` e `*.md`. Os templates `dist-portable-src/`, `dist-dockerdb-src/`,
   `dist-embedded-src/` e `dist-standalone-src/` não estão versionados, e qualquer
   documento novo seria ignorado.

3. **Links quebrados.** `README-FIRST.md` e `README.md` referenciam
   `GUIA-INSTALACAO-DOCKER.md`, `CHECKLIST-INSTALACAO.md`, `DIAGRAMA-DOCKER.md` e
   `QUICKSTART-DOCKER.md`. Nenhum existe.

4. **Porta divergente.** A aplicação escuta em 7070 (`configs/config.yaml`), o
   `docker-compose.yml` publica 8080 e o `Dockerfile` declara `EXPOSE 8080`. A
   documentação cita as duas.

5. **Logs no lugar errado.** `internal/trace/tracer.go` grava em `traces/trace.log` e
   `traces/trace.html`. Todos os scripts de pacote criam uma pasta `logs/` que nunca
   recebe nada.

6. **PostgreSQL portátil manual.** `dist-portable-src/README.txt` instrui o usuário a
   baixar o ZIP do EnterpriseDB e copiar pastas à mão. Bloqueio total para o perfil de
   usuário alvo.

7. **Postgres órfão.** `dist-portable-src/start.bat` só encerra o Postgres quando a
   aplicação termina normalmente. Fechar a janela deixa o processo rodando.

8. **Credenciais reais commitadas.** `configs/config.yaml` contém host, usuário e senha
   de produção do banco W_Access.

9. **Mil portas publicadas.** `docker-compose.yml` publica `4000-4999`, criando mil
   proxies no Docker Desktop e tornando a subida lenta.

10. **Host de banco inválido como cliente.** `configs/config.yaml` usa `host: "0.0.0.0"`
    para `service_db` e `emulator_db`, que são conexões de saída.

## Escopo

Deriva de `new_tasks.md`:

| Item | Decisão |
|---|---|
| 1. Versões Docker e WSL | Dentro. WSL é atendido pelo pacote Linux, não por um pacote próprio. |
| 2. Modo online e standalone | Dentro, restrito ao modo de autenticação do dispositivo. |
| 3. Compatibilidade com Gerenciador GO | **Fora.** O emulador é independente de quem o consome. |
| 4. Manual de instalação e testes | Dentro. |
| 5. Caminho dos logs no manual | Dentro. |

Fora de escopo: instalador `.exe` (Inno Setup/NSIS), serviço do Windows, cadastro
manual de dispositivos sem WXS, e qualquer refatoração não exigida pelos itens acima.

## Arquitetura

Três pacotes ZIP, um por cenário. O usuário extrai e roda; não escolhe modo, não edita
YAML, não instala dependência à mão.

| Pacote | Alvo | Pré-requisito |
|---|---|---|
| `GoFacialEmulator-docker.zip` | Windows com Docker Desktop, Linux, WSL2 | Docker rodando |
| `GoFacialEmulator-windows.zip` | Windows sem Docker | nenhum |
| `GoFacialEmulator-linux.zip` | Servidor Linux e WSL2 | sudo, apt |

Os cinco pacotes atuais e os quatro diretórios `dist-*-src/` são removidos.

### Estrutura interna

Regra comum: na raiz do ZIP ficam apenas os arquivos que o usuário aciona (`INSTALAR`,
`INICIAR`, `PARAR`, `LEIA-ME.txt`). Todo o resto fica em `sistema/`.

```
GoFacialEmulator-docker.zip                 (~45 MB)
├── INSTALAR.bat / instalar.sh
├── INICIAR.bat  / iniciar.sh
├── PARAR.bat    / parar.sh
├── LEIA-ME.txt
└── sistema/
    ├── gofacialemulator-imagem.tar
    ├── docker-compose.yml
    └── logs/

GoFacialEmulator-windows.zip                (~160 MB)
├── INSTALAR.bat
├── INICIAR.bat
├── PARAR.bat
├── LEIA-ME.txt
└── sistema/
    ├── emulator-service.exe
    ├── postgres/                           binários EnterpriseDB (bin, lib, share)
    ├── configs/config.yaml
    └── logs/

GoFacialEmulator-linux.zip                  (~15 MB)
├── instalar.sh
├── iniciar.sh
├── parar.sh
├── LEIA-ME.txt
└── sistema/
    ├── emulator-service
    ├── configs/config.yaml
    └── logs/
```

O pacote Docker leva a imagem pré-construída (`docker save` no build, `docker load` na
instalação) em vez de `build: .`. Isso elimina do cliente o build de alguns minutos, a
necessidade de internet e a presença do código-fonte inteiro no ZIP.

### Contrato dos scripts

Os três scripts têm o mesmo contrato nos três pacotes; só a implementação muda.

**`INSTALAR`** — idempotente. Rodar novamente não destrói dados nem falha.

1. Verifica pré-requisitos e aborta com causa em português se faltar algo.
2. Prepara o banco (carrega imagem e sobe compose / `initdb` local / `apt install`).
3. Cria usuário `emulator` e banco `emulator_db` se ainda não existirem.
4. Grava `sistema/configs/config.yaml` a partir do template, se ainda não existir.
5. Última linha: `✅ Instalado. Rode INICIAR.` ou `❌ <causa> — veja sistema/logs/instalacao.log`.

O schema continua sendo criado por `database.ValidateDatabaseOnStartup` no primeiro
boot; `INSTALAR` não replica essa lógica.

**`INICIAR`**

1. Verifica pré-requisitos (Docker ativo, porta 7070 livre, banco instalado).
2. Sobe o banco e aguarda ficar pronto.
3. Sobe a aplicação.
4. Consulta `GET /health`.
5. Última linha: `✅ Rodando em http://localhost:7070` ou o erro com o que fazer.

**`PARAR`** — encerra aplicação e banco. No pacote Windows resolve o Postgres órfão,
parando o cluster mesmo quando a janela da aplicação foi fechada.

### Detecção de WSL

`instalar.sh` detecta WSL lendo `/proc/version`. A diferença de comportamento:

- Com systemd disponível: unit `gofacialemulator.service`.
- Sem systemd (WSL2 padrão): `service postgresql start` e a aplicação via `nohup`,
  com o PID em `sistema/logs/app.pid` para o `parar.sh` usar.

Um script, dois ambientes.

### Build

Um `build-pacotes.bat [docker|windows|linux|todos]` substitui os cinco scripts atuais.

- Compila `windows/amd64` e `linux/amd64` a partir de `cmd/emulator-service/main.go`.
- Constrói e exporta a imagem Docker (`docker build` + `docker save`).
- Baixa os binários do PostgreSQL para `.build-cache/postgres-portable/` na primeira
  execução e reusa nas seguintes. `.build-cache/` entra no `.gitignore`.
- Monta cada `sistema/` e gera o ZIP correspondente.

Não copia `web/` nem `internal/database/migrations/` — esse conteúdo está embutido no
binário via `go:embed` (`assets/assets.go`).

## Mudanças no código

Limitadas ao que impede um pacote de funcionar ou de ser distribuído.

| Arquivo | Mudança | Motivo |
|---|---|---|
| `internal/trace/tracer.go` | `FOLDER_NAME` de `traces` para `logs` | Uma resposta única para "onde estão os logs" (defeito 5) |
| `configs/config.yaml` | Remover credenciais do W_Access; host `0.0.0.0` → `127.0.0.1`; porta 7070 | Defeitos 4, 8, 10 |
| `docker-compose.yml` | Porta 7070; range `4000-4099`; não publicar 5432 | Defeitos 4, 9 |
| `Dockerfile` | `EXPOSE 7070` | Defeito 4 |
| `.gitignore` | Parar de ignorar `*.md`, `build-*.bat` e o diretório de packaging; ignorar `.build-cache/` | Defeito 2 |
| `internal/handlers/` + template de devices | Botão por dispositivo para alternar `LocalAuthentication` | Item 2 do `new_tasks.md` |

As credenciais do W_Access passam a ser configuradas apenas pela tela `/settings`, que
já grava no banco e tem precedência sobre o YAML (`cmd/emulator-service/main.go`,
carregamento de `GetWxsSettingsFromDB` com fallback para o arquivo).

O range publicado passa a ser `4000-4099` (100 emuladores). O `docker-compose.yml`
carrega a linha `4000-4999` comentada logo acima, para quem precisar de mais.

### Item 2 — online e standalone

Os dois modos já existem e são acionados pelo valor `LocalAuthentication` em
`device_settings`:

- `0` = online: o Site Controller valida o acesso (Hikvision via `remoteCheck`,
  Dahua conforme `dahua/emulator.go`).
- `1` = standalone: o dispositivo valida localmente e gera o evento sozinho.

Hoje esse valor só muda quando o Site Controller escreve em `acsCfg`
(`hikvision/handlers.go`) ou no equivalente Dahua (`dahua/handlers.go`). A mudança é
expor o toggle por dispositivo na interface, com endpoint próprio, para que o modo possa
ser testado sem depender do SC. O comportamento dos emuladores não muda.

## Documentação

`MANUAL.md`, arquivo único, nesta ordem:

1. **Qual pacote usar** — três linhas, uma por cenário.
2. **Instalação** — uma seção por pacote, com o caminho exato onde extrair.
3. **Primeiro uso** — configurar o WXS em `/settings`, Refresh DB, Start.
4. **Como rodar os testes** — `go test ./...` no repositório e a verificação manual dos
   endpoints de um emulador (`/ISAPI/System/deviceInfo`, `alertStream`).
5. **Onde estão os logs** — tabela por pacote.
6. **Problemas comuns** — porta ocupada, Docker parado, WXS inacessível.

Caminhos de log documentados:

| Pacote | Aplicação | PostgreSQL |
|---|---|---|
| Windows | `sistema\logs\trace.log` e `trace.html` | `sistema\logs\postgres.log` |
| Docker | `sistema/logs/trace.log` (volume) ou `docker compose logs -f app` | `docker compose logs -f postgres` |
| Linux / WSL | `sistema/logs/trace.log` | `/var/log/postgresql/` |

`README.md` vira índice curto apontando para o `MANUAL.md`. `README-FIRST.md` é
removido — seus quatro destinos não existem. `DOCKER.md`, `PORTAS-EMULADORES.md` e
`MONITORING.md` permanecem como anexos técnicos, referenciados pelo manual, não como
porta de entrada.

## Verificação

- Cada ZIP é extraído numa máquina ou VM limpa e instalado apenas com os scripts da
  raiz, sem passo manual.
- Após `INICIAR`, `GET http://localhost:7070/health` responde com sucesso.
- `INSTALAR` executado duas vezes seguidas não falha nem apaga dados.
- `PARAR` no pacote Windows encerra o Postgres, inclusive após a janela da aplicação ter
  sido fechada.
- `go test ./...` passa.
- A pasta `logs/` de cada pacote contém `trace.log` após o primeiro start.
- `configs/config.yaml` versionado não contém nenhuma credencial real.
- Nenhum documento restante aponta para arquivo inexistente.
