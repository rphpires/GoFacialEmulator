# Instalação Simplificada — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Substituir os cinco pacotes de instalação atuais (três deles quebrados) por três ZIPs auto-contidos — Docker, Windows sem Docker e Linux/WSL — cada um instalável por um técnico leigo com um duplo-clique, acompanhados de um manual único.

**Architecture:** Um script `packaging/build-pacotes.bat` gera os três ZIPs a partir de templates versionados em `packaging/<alvo>/`. Cada ZIP expõe na raiz apenas `INSTALAR`, `INICIAR`, `PARAR` e `LEIA-ME.txt`; o resto fica em `sistema/`. O pacote Docker leva a imagem pré-construída via `docker save`, o pacote Windows leva os binários do PostgreSQL embutidos, e o pacote Linux serve também WSL2 por detecção em `/proc/version`. Mudanças no código Go ficam restritas ao que impede um pacote de funcionar ou de ser distribuído.

**Tech Stack:** Go 1.21 (toolchain local 1.25.4), Gin, pgx/v5, PostgreSQL 15, Docker Compose v2, batch (`cmd.exe`) e bash.

**Spec:** `docs/superpowers/specs/2026-08-13-instalacao-simplificada-design.md`

## Global Constraints

- Porta HTTP da aplicação: **7070** em todos os arquivos, sem exceção.
- Porta do PostgreSQL: **5432** no Docker e no Linux; **5433** no pacote Windows portátil.
- Credenciais do banco local da aplicação: usuário `emulator`, senha `emulator123`, banco `emulator_db`.
- Range de portas de emuladores publicado por padrão: **4000-4099**.
- Nenhum arquivo versionado pode conter credencial real do W_Access. Strings proibidas: `db_W-X-S@Wellcare924_`, `172.16.17.67`, `172.20.112.1`.
- Diretório de logs da aplicação: `logs` (não `traces`).
- Toda mensagem que o usuário final lê é em português, sem jargão. Os scripts do pacote terminam com uma linha `✅ ...` ou `❌ ... — veja sistema/logs/instalacao.log`.
- Scripts `INSTALAR` são idempotentes: rodar duas vezes não falha e não apaga dados.
- Assets web e migrations são embutidos no binário via `go:embed` (`assets/assets.go`). Nenhum script de build copia `web/` ou `internal/database/migrations/` — essas pastas não existem.
- Commits em português no corpo quando descreverem comportamento para o usuário final; título em inglês seguindo Conventional Commits, como no histórico do repositório.

---

### Task 1: Logs em `logs/` em vez de `traces/`

O `Tracer` grava em `traces/trace.log`, mas todo script de pacote cria `logs/`. O manual precisa de uma resposta única para "onde estão os logs".

**Files:**
- Modify: `internal/trace/tracer.go:16`
- Modify: `docker-compose.yml:57-58`
- Modify: `docker-compose.dev.yml`
- Modify: `docker-compose.deploy.yml:48-49`
- Modify: `Dockerfile:34`
- Modify: `.gitignore`
- Test: `internal/trace/tracer_folder_test.go` (criar)

**Interfaces:**
- Consumes: nada.
- Produces: `trace.FOLDER_NAME == "logs"`. As tarefas 4, 5 e 6 assumem que a aplicação cria e escreve em `logs/` relativo ao diretório de trabalho.

- [ ] **Step 1: Escrever o teste que falha**

Criar `internal/trace/tracer_folder_test.go`. O teste roda `init()` diretamente (mesmo pacote, sem passar pelo singleton `NewTracer`) num diretório temporário:

```go
package trace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInitCreatesLogsFolder garante que o tracer grava em "logs/", que é o
// caminho documentado no MANUAL.md e criado pelos pacotes de instalação.
func TestInitCreatesLogsFolder(t *testing.T) {
	dir := t.TempDir()

	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	tr := &Tracer{}
	if err := tr.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(tr.Close)

	if FOLDER_NAME != "logs" {
		t.Errorf("FOLDER_NAME = %q, quero %q", FOLDER_NAME, "logs")
	}

	logFile := filepath.Join(dir, "logs", "trace.log")
	if _, err := os.Stat(logFile); err != nil {
		t.Errorf("esperava %s criado, erro: %v", logFile, err)
	}

	if _, err := os.Stat(filepath.Join(dir, "traces")); err == nil {
		t.Error("pasta traces/ não deveria mais ser criada")
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `go test ./internal/trace/ -run TestInitCreatesLogsFolder -v`
Expected: FAIL — `FOLDER_NAME = "traces", quero "logs"` e `esperava .../logs/trace.log criado`.

- [ ] **Step 3: Trocar a constante**

Em `internal/trace/tracer.go`, linha 16:

```go
	// FOLDER_NAME is the name of the folder where logs are stored
	FOLDER_NAME = "logs"
```

- [ ] **Step 4: Rodar o teste e confirmar que passa**

Run: `go test ./internal/trace/ -run TestInitCreatesLogsFolder -v`
Expected: PASS

- [ ] **Step 5: Alinhar os volumes do Docker e o Dockerfile**

Em `docker-compose.yml`, `docker-compose.dev.yml` e `docker-compose.deploy.yml`, substituir o par de volumes da aplicação por um só:

```yaml
    volumes:
      - ./logs:/app/logs
```

Em `Dockerfile`, linha 34:

```dockerfile
RUN mkdir -p logs
```

Em `.gitignore`, remover as três entradas que só existiam para a pasta antiga (`traces/`, `/traces`, `*.trace`) e manter `logs/`.

- [ ] **Step 6: Rodar a suíte inteira**

Run: `go test ./...`
Expected: PASS (o pacote `testes/` é um módulo separado e não entra nessa execução)

- [ ] **Step 7: Commit**

```bash
git add internal/trace/tracer.go internal/trace/tracer_folder_test.go docker-compose.yml docker-compose.dev.yml docker-compose.deploy.yml Dockerfile .gitignore
git commit -m "$(cat <<'EOF'
fix(trace): write logs to logs/ instead of traces/

Todo script de pacote criava uma pasta logs/ que nunca recebia nada,
enquanto o tracer gravava em traces/. Uma pasta só, para o manual poder
apontar um caminho único de log por pacote.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Tirar credenciais reais do repositório e unificar a porta em 7070

`configs/config.yaml` e a migration V001 carregam host, usuário e senha de produção do W_Access. Além disso a aplicação escuta em 7070 enquanto o compose publica 8080.

**Files:**
- Modify: `configs/config.yaml`
- Modify: `configs/config.local.yaml`
- Modify: `assets/migrations/V001_create_emulator_schema.sql:197-200`
- Modify: `docker-compose.yml`
- Modify: `docker-compose.deploy.yml`
- Modify: `docker-compose.dev.yml`
- Modify: `Dockerfile:37`
- Test: `internal/config/config_secrets_test.go` (criar)

**Interfaces:**
- Consumes: nada.
- Produces: `configs/config.yaml` com `server.port: 7070`, `service_db.host: "127.0.0.1"`, `service_db.port: 5432` e bloco `wxsDB` sem credencial. As tarefas 4, 5 e 6 copiam esse arquivo como template.

- [ ] **Step 1: Escrever o teste que falha**

Criar `internal/config/config_secrets_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// segredosProibidos são credenciais reais do W_Access que já vazaram para
// arquivos versionados. Nenhum arquivo distribuído pode contê-las.
var segredosProibidos = []string{
	"db_W-X-S@Wellcare924_",
	"172.16.17.67",
	"172.20.112.1",
}

var arquivosVersionados = []string{
	"../../configs/config.yaml",
	"../../configs/config.local.yaml",
	"../../assets/migrations/V001_create_emulator_schema.sql",
}

func TestArquivosVersionadosNaoContemCredenciais(t *testing.T) {
	for _, arquivo := range arquivosVersionados {
		conteudo, err := os.ReadFile(arquivo)
		if err != nil {
			t.Fatalf("lendo %s: %v", arquivo, err)
		}
		for _, segredo := range segredosProibidos {
			if strings.Contains(string(conteudo), segredo) {
				t.Errorf("%s contém credencial proibida %q", filepath.Base(arquivo), segredo)
			}
		}
	}
}

func TestConfigYamlUsaPorta7070ELocalhost(t *testing.T) {
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 7070 {
		t.Errorf("server.port = %d, quero 7070", cfg.Server.Port)
	}
	if cfg.ServiceDB.Host != "127.0.0.1" {
		t.Errorf("service_db.host = %q, quero \"127.0.0.1\"", cfg.ServiceDB.Host)
	}
}
```

- [ ] **Step 2: Rodar o teste e confirmar que falha**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `config.yaml contém credencial proibida "db_W-X-S@Wellcare924_"` e `service_db.host = "0.0.0.0", quero "127.0.0.1"`.

Nota: `Load` lê `.env` do diretório de trabalho do teste (`internal/config/`), onde não existe `.env`, então nenhum override interfere.

- [ ] **Step 3: Reescrever `configs/config.yaml`**

```yaml
app_version: "1.0.0"

server:
  host: "0.0.0.0"
  port: 7070

service_db:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5432
  username: "emulator"
  password: "emulator123"
  database: "emulator_db"
  sslmode: "disable"
  max_connections: 10
  max_idle_connections: 5

emulator_db:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5432
  username: "emulator"
  password: "emulator123"
  database: "emulator_db"
  sslmode: "disable"
  max_connections: 20
  max_idle_connections: 10

# Banco do W-Access (sistema externo).
# NÃO preencher aqui. Configure pela tela http://localhost:7070/settings,
# que grava no banco e tem precedência sobre este arquivo.
wxsDB:
  driver: "mssql"
  host: ""
  port: 1433
  database: "W_Access"
  username: ""
  password: ""
  timeout: "30s"

logging:
  level: "info"
  format: "json"
  enable_trace: true
  max_file_size: "50MB"
  max_files: 10
```

Aplicar o mesmo tratamento ao bloco `wxsDB` de `configs/config.local.yaml`, preservando o restante desse arquivo como está.

- [ ] **Step 4: Remover o seed de credenciais da migration**

Em `assets/migrations/V001_create_emulator_schema.sql`, substituir o bloco das linhas 197-200 por:

```sql
-- Configuração do WXS: sem seed de credenciais.
-- O usuário preenche host, banco, usuário e senha na tela /settings.
```

- [ ] **Step 5: Rodar o teste e confirmar que passa**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 6: Unificar a porta nos arquivos Docker**

Em `docker-compose.yml`, `docker-compose.dev.yml` e `docker-compose.deploy.yml`, no serviço `app`: trocar `SERVER_PORT: "8080"` por `SERVER_PORT: "7070"`, e o mapeamento `"8080:8080"` por `"7070:7070"`. Trocar o range de portas por:

```yaml
      # 100 emuladores. Para mais, troque pela linha abaixo:
      # - "4000-4999:4000-4999"
      - "4000-4099:4000-4099"
```

Em `Dockerfile`, linha 37:

```dockerfile
EXPOSE 7070
```

- [ ] **Step 7: Verificar que nenhuma porta 8080 sobrou nos arquivos de execução**

Run: `git grep -n "8080" -- docker-compose*.yml Dockerfile configs/`
Expected: nenhuma saída

- [ ] **Step 8: Commit**

```bash
git add configs/ assets/migrations/V001_create_emulator_schema.sql docker-compose.yml docker-compose.dev.yml docker-compose.deploy.yml Dockerfile internal/config/config_secrets_test.go
git commit -m "$(cat <<'EOF'
fix(config): remove W_Access credentials and unify HTTP port on 7070

Host, usuário e senha de produção do W_Access estavam em configs/config.yaml
e no seed da migration V001. A configuração passa a ser feita apenas pela
tela /settings, que já grava no banco e tem precedência sobre o YAML.

A aplicação escuta em 7070 mas o compose publicava 8080; agora é 7070 em
todos os arquivos. Range de emuladores publicado cai de 1000 para 100 portas,
com a linha antiga comentada para quem precisar.

A senha exposta continua no histórico do git — rotacioná-la no W_Access é
uma ação separada, fora deste plano.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Versionar packaging e documentação, remover artefatos mortos

`.gitignore` ignora `*.md`, `build-*.bat` e `dist-*/`. Sem corrigir isso, nada das tarefas seguintes pode ser commitado.

**Files:**
- Modify: `.gitignore`
- Delete: `build-dist.bat`, `build-dockerdb.bat`, `build-embedded.bat`, `build-portable.bat`, `build-standalone.bat`
- Delete: `dist-dockerdb-src/`, `dist-embedded-src/`, `dist-portable-src/`, `dist-standalone-src/`
- Delete: `README-FIRST.md`, `internal/emulator/manager.go.bak`
- Create: `packaging/.gitkeep`

**Interfaces:**
- Consumes: nada.
- Produces: `packaging/` versionado; `docs/**/*.md` e `MANUAL.md` versionáveis. As tarefas 4 a 8 dependem disso.

- [ ] **Step 1: Corrigir o `.gitignore`**

Remover as linhas `CLAUDE.md`, `*.md`, `build-*.bat` e `dist-*/`. Acrescentar, no lugar delas:

```gitignore
# Documentação local do Claude (o restante dos .md é versionado)
CLAUDE.md

# Saída de build dos pacotes
packaging/.out/
.build-cache/
```

- [ ] **Step 2: Confirmar que os documentos passam a ser vistos pelo git**

Run: `git check-ignore -v MANUAL.md docs/superpowers/plans/2026-08-13-instalacao-simplificada.md packaging/.gitkeep`
Expected: nenhuma saída (nenhum dos três é ignorado)

- [ ] **Step 3: Apagar os build scripts e templates antigos**

```bash
rm -f build-dist.bat build-dockerdb.bat build-embedded.bat build-portable.bat build-standalone.bat
rm -rf dist-dockerdb-src dist-embedded-src dist-portable-src dist-standalone-src
rm -f README-FIRST.md internal/emulator/manager.go.bak
mkdir -p packaging && touch packaging/.gitkeep
```

`README-FIRST.md` aponta para quatro arquivos que não existem e é substituído pelo `MANUAL.md` na Task 8. `manager.go.bak` é um backup versionado por engano.

- [ ] **Step 4: Confirmar que o projeto ainda compila e os testes passam**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add -A .gitignore build-dist.bat build-dockerdb.bat build-embedded.bat build-portable.bat build-standalone.bat dist-dockerdb-src dist-embedded-src dist-portable-src dist-standalone-src README-FIRST.md internal/emulator/manager.go.bak packaging/.gitkeep
git commit -m "$(cat <<'EOF'
chore: version packaging and docs, drop dead build artifacts

.gitignore ignorava *.md, build-*.bat e dist-*/, deixando fora do git toda a
camada de empacotamento e qualquer documento novo.

Os cinco build scripts e os quatro dist-*-src/ saem: três deles copiavam
web/ e internal/database/migrations/, pastas que deixaram de existir no
commit 13acc36 (go:embed), e produziam pacotes incompletos. Substituídos
pelo packaging/ nas tarefas seguintes.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Pacote Docker

Primeiro dos três pacotes. Estabelece o contrato `INSTALAR`/`INICIAR`/`PARAR` que os outros dois seguem.

**Files:**
- Create: `packaging/docker/docker-compose.yml`
- Create: `packaging/docker/INSTALAR.bat`
- Create: `packaging/docker/INICIAR.bat`
- Create: `packaging/docker/PARAR.bat`
- Create: `packaging/docker/instalar.sh`
- Create: `packaging/docker/iniciar.sh`
- Create: `packaging/docker/parar.sh`
- Create: `packaging/docker/LEIA-ME.txt`
- Create: `packaging/build-pacotes.bat`

**Interfaces:**
- Consumes: `configs/config.yaml` da Task 2; `logs/` da Task 1.
- Produces: `packaging/build-pacotes.bat [docker|windows|linux|todos]`, que escreve os ZIPs em `packaging/.out/`. As tarefas 5 e 6 acrescentam alvos a esse mesmo script. Nome e tag da imagem: `gofacialemulator:1.0`.

- [ ] **Step 1: Escrever o compose do pacote**

`packaging/docker/docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:15-alpine
    container_name: facial-emulator-db
    environment:
      POSTGRES_USER: emulator
      POSTGRES_PASSWORD: emulator123
      POSTGRES_DB: emulator_db
    expose:
      - "5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U emulator -d emulator_db"]
      interval: 5s
      timeout: 3s
      retries: 20
    restart: unless-stopped

  app:
    image: gofacialemulator:1.0
    container_name: facial-emulator-app
    environment:
      SERVER_HOST: "0.0.0.0"
      SERVER_PORT: "7070"
      DB_HOST: postgres
      DB_PORT: "5432"
      DB_USER: emulator
      DB_PASSWORD: emulator123
      DB_DATABASE: emulator_db
    ports:
      - "7070:7070"
      # 100 emuladores. Para mais, troque pela linha abaixo:
      # - "4000-4999:4000-4999"
      - "4000-4099:4000-4099"
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      default:
        aliases:
          - app
    restart: unless-stopped
    volumes:
      - ./logs:/app/logs

volumes:
  postgres_data:
    driver: local
```

O banco não publica porta no host: evita conflito com um PostgreSQL já instalado na máquina do cliente.

- [ ] **Step 2: Escrever `INSTALAR.bat`**

`packaging/docker/INSTALAR.bat`:

```bat
@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title GoFacialEmulator - Instalacao

if not exist sistema\logs mkdir sistema\logs
set "LOG=sistema\logs\instalacao.log"

echo. > "%LOG%"
echo ==============================================================
echo   GoFacialEmulator - Instalacao (Docker)
echo ==============================================================
echo.

echo [1/3] Verificando o Docker ...
docker info >nul 2>>"%LOG%"
if errorlevel 1 (
    echo.
    echo ^❌ O Docker nao esta rodando.
    echo    Abra o Docker Desktop, espere o icone ficar verde e rode
    echo    este INSTALAR.bat de novo.
    echo.
    pause
    exit /b 1
)
echo       Docker OK.

echo [2/3] Carregando a aplicacao ...
docker load -i sistema\gofacialemulator-imagem.tar >>"%LOG%" 2>&1
if errorlevel 1 (
    echo.
    echo ^❌ Falha ao carregar a aplicacao — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)
echo       Aplicacao carregada.

echo [3/3] Preparando o banco de dados ...
docker compose -f sistema\docker-compose.yml up -d postgres >>"%LOG%" 2>&1
if errorlevel 1 (
    echo.
    echo ^❌ Falha ao preparar o banco — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)
echo       Banco preparado.

echo.
echo ^✅ Instalado. Rode INICIAR.bat
echo.
pause
```

- [ ] **Step 3: Escrever `INICIAR.bat`**

`packaging/docker/INICIAR.bat`:

```bat
@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title GoFacialEmulator

if not exist sistema\logs mkdir sistema\logs
set "LOG=sistema\logs\instalacao.log"

echo ==============================================================
echo   GoFacialEmulator - Iniciando
echo ==============================================================
echo.

docker info >nul 2>&1
if errorlevel 1 (
    echo ^❌ O Docker nao esta rodando. Abra o Docker Desktop e tente de novo.
    echo.
    pause
    exit /b 1
)

docker image inspect gofacialemulator:1.0 >nul 2>&1
if errorlevel 1 (
    echo ^❌ A aplicacao ainda nao foi instalada. Rode INSTALAR.bat primeiro.
    echo.
    pause
    exit /b 1
)

echo [1/2] Subindo os servicos ...
docker compose -f sistema\docker-compose.yml up -d >>"%LOG%" 2>&1
if errorlevel 1 (
    echo ^❌ Falha ao subir os servicos — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)

echo [2/2] Aguardando a aplicacao responder ...
set /a tentativas=0
:esperar
set /a tentativas+=1
curl -sf http://localhost:7070/monitoring/health/quick >nul 2>&1
if not errorlevel 1 goto pronto
if %tentativas% geq 60 (
    echo.
    echo ^❌ A aplicacao nao respondeu em 60 segundos.
    echo    Veja o log com: docker compose -f sistema\docker-compose.yml logs app
    echo.
    pause
    exit /b 1
)
timeout /t 1 /nobreak >nul
goto esperar

:pronto
echo.
echo ^✅ Rodando em http://localhost:7070
echo.
echo    Para parar: PARAR.bat
echo.
pause
```

- [ ] **Step 4: Escrever `PARAR.bat`**

`packaging/docker/PARAR.bat`:

```bat
@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title GoFacialEmulator - Parando

echo Parando o GoFacialEmulator ...
docker compose -f sistema\docker-compose.yml stop >nul 2>&1
echo.
echo ^✅ Parado. Os dados continuam salvos.
echo.
pause
```

- [ ] **Step 5: Escrever os equivalentes em bash**

`packaging/docker/instalar.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log
: > "$LOG"

echo "=============================================================="
echo "  GoFacialEmulator - Instalacao (Docker)"
echo "=============================================================="

echo "[1/3] Verificando o Docker ..."
if ! docker info >>"$LOG" 2>&1; then
    echo
    echo "❌ O Docker nao esta rodando ou seu usuario nao tem permissao."
    echo "   Inicie o Docker e rode ./instalar.sh de novo."
    exit 1
fi
echo "      Docker OK."

echo "[2/3] Carregando a aplicacao ..."
if ! docker load -i sistema/gofacialemulator-imagem.tar >>"$LOG" 2>&1; then
    echo
    echo "❌ Falha ao carregar a aplicacao — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Aplicacao carregada."

echo "[3/3] Preparando o banco de dados ..."
if ! docker compose -f sistema/docker-compose.yml up -d postgres >>"$LOG" 2>&1; then
    echo
    echo "❌ Falha ao preparar o banco — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Banco preparado."

echo
echo "✅ Instalado. Rode ./iniciar.sh"
```

`packaging/docker/iniciar.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log

echo "=============================================================="
echo "  GoFacialEmulator - Iniciando"
echo "=============================================================="

if ! docker info >/dev/null 2>&1; then
    echo "❌ O Docker nao esta rodando. Inicie o Docker e tente de novo."
    exit 1
fi

if ! docker image inspect gofacialemulator:1.0 >/dev/null 2>&1; then
    echo "❌ A aplicacao ainda nao foi instalada. Rode ./instalar.sh primeiro."
    exit 1
fi

echo "[1/2] Subindo os servicos ..."
if ! docker compose -f sistema/docker-compose.yml up -d >>"$LOG" 2>&1; then
    echo "❌ Falha ao subir os servicos — veja sistema/logs/instalacao.log"
    exit 1
fi

echo "[2/2] Aguardando a aplicacao responder ..."
for _ in $(seq 1 60); do
    if curl -sf http://localhost:7070/monitoring/health/quick >/dev/null 2>&1; then
        echo
        echo "✅ Rodando em http://localhost:7070"
        echo
        echo "   Para parar: ./parar.sh"
        exit 0
    fi
    sleep 1
done

echo
echo "❌ A aplicacao nao respondeu em 60 segundos."
echo "   Veja o log com: docker compose -f sistema/docker-compose.yml logs app"
exit 1
```

`packaging/docker/parar.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

echo "Parando o GoFacialEmulator ..."
docker compose -f sistema/docker-compose.yml stop >/dev/null 2>&1
echo
echo "✅ Parado. Os dados continuam salvos."
```

- [ ] **Step 6: Escrever o `LEIA-ME.txt`**

`packaging/docker/LEIA-ME.txt`:

```
GoFacialEmulator - versao Docker

O QUE FAZER
  1. Abra o Docker Desktop e espere ficar verde.
  2. Duplo-clique em INSTALAR.bat  (so na primeira vez)
  3. Duplo-clique em INICIAR.bat
  4. Abra http://localhost:7070 no navegador.
  Para parar: PARAR.bat

  No Linux, use ./instalar.sh, ./iniciar.sh e ./parar.sh.

LOGS
  Aplicacao: sistema/logs/trace.log  (e trace.html, colorido)
  Banco:     docker compose -f sistema/docker-compose.yml logs postgres

O manual completo esta no arquivo MANUAL.md do projeto.
```

- [ ] **Step 7: Escrever o `build-pacotes.bat` com o alvo `docker`**

`packaging/build-pacotes.bat`:

```bat
@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul
cd /d "%~dp0.."

set "ALVO=%~1"
if "%ALVO%"=="" set "ALVO=todos"

set "OUT=packaging\.out"
if not exist "%OUT%" mkdir "%OUT%"

where go >nul 2>&1
if errorlevel 1 (
    echo [ERRO] Go nao encontrado. Instale em https://go.dev/dl/
    exit /b 1
)

if /i "%ALVO%"=="docker"  goto build_docker
if /i "%ALVO%"=="windows" goto build_windows
if /i "%ALVO%"=="linux"   goto build_linux
if /i "%ALVO%"=="todos"   goto build_docker
echo [ERRO] Alvo invalido: %ALVO%
echo Uso: build-pacotes.bat [docker^|windows^|linux^|todos]
exit /b 1

REM ==================== DOCKER ====================
:build_docker
echo.
echo [docker] Construindo a imagem ...
docker build -t gofacialemulator:1.0 .
if errorlevel 1 (
    echo [ERRO] docker build falhou.
    exit /b 1
)

set "STAGE=%OUT%\docker"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%\sistema\logs"

echo [docker] Exportando a imagem ...
docker save -o "%STAGE%\sistema\gofacialemulator-imagem.tar" gofacialemulator:1.0
if errorlevel 1 (
    echo [ERRO] docker save falhou.
    exit /b 1
)

copy /Y packaging\docker\docker-compose.yml "%STAGE%\sistema\" >nul
copy /Y packaging\docker\INSTALAR.bat  "%STAGE%\" >nul
copy /Y packaging\docker\INICIAR.bat   "%STAGE%\" >nul
copy /Y packaging\docker\PARAR.bat     "%STAGE%\" >nul
copy /Y packaging\docker\instalar.sh   "%STAGE%\" >nul
copy /Y packaging\docker\iniciar.sh    "%STAGE%\" >nul
copy /Y packaging\docker\parar.sh      "%STAGE%\" >nul
copy /Y packaging\docker\LEIA-ME.txt   "%STAGE%\" >nul

if exist "%OUT%\GoFacialEmulator-docker.zip" del "%OUT%\GoFacialEmulator-docker.zip"
powershell -NoProfile -Command "Compress-Archive -Path '%STAGE%\*' -DestinationPath '%OUT%\GoFacialEmulator-docker.zip' -Force"
if errorlevel 1 (
    echo [ERRO] Falha ao gerar o ZIP.
    exit /b 1
)
echo [docker] OK: %OUT%\GoFacialEmulator-docker.zip
if /i not "%ALVO%"=="todos" goto fim

:fim
echo.
echo Pacotes gerados em %OUT%\
dir /b "%OUT%\*.zip"
exit /b 0
```

- [ ] **Step 8: Gerar o pacote e conferir o conteúdo**

Run: `packaging\build-pacotes.bat docker`
Expected: termina com `[docker] OK: packaging\.out\GoFacialEmulator-docker.zip`

Run: `powershell -NoProfile -Command "Add-Type -A System.IO.Compression.FileSystem; [IO.Compression.ZipFile]::OpenRead((Resolve-Path packaging\.out\GoFacialEmulator-docker.zip)).Entries | Select-Object -Expand FullName"`
Expected: na raiz só `INSTALAR.bat`, `INICIAR.bat`, `PARAR.bat`, `instalar.sh`, `iniciar.sh`, `parar.sh`, `LEIA-ME.txt`; o resto sob `sistema/`.

- [ ] **Step 9: Testar o pacote de ponta a ponta**

Extrair o ZIP numa pasta vazia fora do repositório e, nela:

Run: `INSTALAR.bat`
Expected: última linha `✅ Instalado. Rode INICIAR.bat`

Run: `INICIAR.bat`
Expected: última linha `✅ Rodando em http://localhost:7070`

Run: `curl -sf http://localhost:7070/monitoring/health/quick`
Expected: resposta JSON com `status`

Run: `INSTALAR.bat` (segunda vez, para provar idempotência)
Expected: `✅ Instalado. Rode INICIAR.bat`, sem erro

Run: `PARAR.bat`
Expected: `✅ Parado. Os dados continuam salvos.`

Conferir que `sistema\logs\trace.log` existe e tem conteúdo.

- [ ] **Step 10: Commit**

```bash
git add packaging/
git commit -m "$(cat <<'EOF'
feat(packaging): add self-contained Docker package

Três arquivos na raiz do ZIP — INSTALAR, INICIAR, PARAR — e o resto em
sistema/. A imagem vai pré-construída via docker save, então o cliente não
compila nada nem precisa de internet no momento da instalação.

INICIAR só declara sucesso depois que /monitoring/health/quick responde.
O Postgres não publica porta no host, para não conflitar com uma instalação
existente na máquina do cliente.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Pacote Windows com PostgreSQL embutido

O pacote de maior fricção hoje: o `README.txt` atual manda o usuário baixar binários do EnterpriseDB e copiar pastas à mão.

**Files:**
- Create: `packaging/windows/INSTALAR.bat`
- Create: `packaging/windows/INICIAR.bat`
- Create: `packaging/windows/PARAR.bat`
- Create: `packaging/windows/LEIA-ME.txt`
- Create: `packaging/windows/config.yaml`
- Modify: `packaging/build-pacotes.bat`

**Interfaces:**
- Consumes: `packaging/build-pacotes.bat` da Task 4.
- Produces: `packaging/.out/GoFacialEmulator-windows.zip`. PostgreSQL portátil em `sistema\postgres`, cluster em `sistema\postgres\data`, porta 5433.

- [ ] **Step 1: Escrever o `config.yaml` do pacote**

`packaging/windows/config.yaml` — igual ao `configs/config.yaml` da Task 2, com a porta do banco em 5433:

```yaml
app_version: "1.0.0"

server:
  host: "0.0.0.0"
  port: 7070

service_db:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5433
  username: "emulator"
  password: "emulator123"
  database: "emulator_db"
  sslmode: "disable"
  max_connections: 10
  max_idle_connections: 5

emulator_db:
  driver: "postgres"
  host: "127.0.0.1"
  port: 5433
  username: "emulator"
  password: "emulator123"
  database: "emulator_db"
  sslmode: "disable"
  max_connections: 20
  max_idle_connections: 10

# Banco do W-Access (sistema externo).
# NÃO preencher aqui. Configure pela tela http://localhost:7070/settings.
wxsDB:
  driver: "mssql"
  host: ""
  port: 1433
  database: "W_Access"
  username: ""
  password: ""
  timeout: "30s"

logging:
  level: "info"
  format: "json"
  enable_trace: true
  max_file_size: "50MB"
  max_files: 10
```

- [ ] **Step 2: Escrever `INSTALAR.bat`**

`packaging/windows/INSTALAR.bat`:

```bat
@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title GoFacialEmulator - Instalacao

set "PGBIN=%~dp0sistema\postgres\bin"
set "PGDATA=%~dp0sistema\postgres\data"
set "PGPORT=5433"

if not exist sistema\logs mkdir sistema\logs
set "LOG=%~dp0sistema\logs\instalacao.log"
echo. > "%LOG%"

echo ==============================================================
echo   GoFacialEmulator - Instalacao (Windows)
echo ==============================================================
echo.

if not exist "%PGBIN%\postgres.exe" (
    echo ^❌ Pacote incompleto: falta sistema\postgres\bin\postgres.exe
    echo    Baixe o ZIP do GoFacialEmulator de novo.
    echo.
    pause
    exit /b 1
)

if exist "%PGDATA%\PG_VERSION" (
    echo O banco ja estava instalado. Nada a fazer.
    echo.
    echo ^✅ Instalado. Rode INICIAR.bat
    echo.
    pause
    exit /b 0
)

echo [1/3] Criando o banco de dados ...
set "PWFILE=%TEMP%\gofe_pw.txt"
> "%PWFILE%" echo postgres
"%PGBIN%\initdb.exe" -D "%PGDATA%" -U postgres --pwfile="%PWFILE%" -E UTF8 --locale=C >>"%LOG%" 2>&1
set RC=%ERRORLEVEL%
del /f /q "%PWFILE%" >nul 2>&1
if not %RC%==0 (
    echo.
    echo ^❌ Falha ao criar o banco — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)

powershell -NoProfile -Command "(Get-Content '%PGDATA%\postgresql.conf') -replace '^#?port\s*=.*', 'port = %PGPORT%' | Set-Content '%PGDATA%\postgresql.conf'"

echo [2/3] Ligando o banco ...
"%PGBIN%\pg_ctl.exe" -D "%PGDATA%" -l "%~dp0sistema\logs\postgres.log" -w start >>"%LOG%" 2>&1
if errorlevel 1 (
    echo.
    echo ^❌ Falha ao ligar o banco — veja sistema\logs\postgres.log
    echo.
    pause
    exit /b 1
)

echo [3/3] Criando o usuario da aplicacao ...
set PGPASSWORD=postgres
"%PGBIN%\psql.exe" -h 127.0.0.1 -p %PGPORT% -U postgres -d postgres -v ON_ERROR_STOP=1 ^
  -c "CREATE USER emulator WITH PASSWORD 'emulator123';" ^
  -c "CREATE DATABASE emulator_db OWNER emulator;" >>"%LOG%" 2>&1
set RC=%ERRORLEVEL%
set PGPASSWORD=

"%PGBIN%\pg_ctl.exe" -D "%PGDATA%" -w stop >>"%LOG%" 2>&1

if not %RC%==0 (
    echo.
    echo ^❌ Falha ao criar o usuario — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)

echo.
echo ^✅ Instalado. Rode INICIAR.bat
echo.
pause
```

- [ ] **Step 3: Escrever `INICIAR.bat`**

Roda a aplicação em janela própria minimizada e só declara sucesso depois do health check, para que fechar a janela do console de instalação não derrube nada pela metade.

`packaging/windows/INICIAR.bat`:

```bat
@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0sistema"
title GoFacialEmulator

set "PGBIN=%~dp0sistema\postgres\bin"
set "PGDATA=%~dp0sistema\postgres\data"

if not exist logs mkdir logs

echo ==============================================================
echo   GoFacialEmulator - Iniciando
echo ==============================================================
echo.

if not exist "%PGDATA%\PG_VERSION" (
    echo ^❌ O banco ainda nao foi instalado. Rode INSTALAR.bat primeiro.
    echo.
    pause
    exit /b 1
)

netstat -ano | findstr /r /c:":7070 .*LISTENING" >nul 2>&1
if not errorlevel 1 (
    echo ^❌ A porta 7070 ja esta em uso.
    echo    O emulador pode ja estar rodando: abra http://localhost:7070
    echo    Se nao for ele, rode PARAR.bat e tente de novo.
    echo.
    pause
    exit /b 1
)

echo [1/3] Ligando o banco ...
"%PGBIN%\pg_ctl.exe" -D "%PGDATA%" -l "logs\postgres.log" -w start >nul 2>&1
if errorlevel 1 (
    echo ^❌ Falha ao ligar o banco — veja sistema\logs\postgres.log
    echo.
    pause
    exit /b 1
)

echo [2/3] Iniciando a aplicacao ...
start "GoFacialEmulator" /min emulator-service.exe -config configs\config.yaml

echo [3/3] Aguardando a aplicacao responder ...
set /a tentativas=0
:esperar
set /a tentativas+=1
curl -sf http://localhost:7070/monitoring/health/quick >nul 2>&1
if not errorlevel 1 goto pronto
if %tentativas% geq 60 (
    echo.
    echo ^❌ A aplicacao nao respondeu em 60 segundos.
    echo    Veja sistema\logs\trace.log
    echo.
    pause
    exit /b 1
)
timeout /t 1 /nobreak >nul
goto esperar

:pronto
echo.
echo ^✅ Rodando em http://localhost:7070
echo.
echo    Para parar: PARAR.bat
echo.
pause
```

- [ ] **Step 4: Escrever `PARAR.bat`**

Encerra a aplicação e o banco, inclusive quando a janela da aplicação já foi fechada — o defeito do pacote portátil atual.

`packaging/windows/PARAR.bat`:

```bat
@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title GoFacialEmulator - Parando

set "PGBIN=%~dp0sistema\postgres\bin"
set "PGDATA=%~dp0sistema\postgres\data"

echo Parando a aplicacao ...
taskkill /IM emulator-service.exe /F >nul 2>&1

echo Parando o banco ...
"%PGBIN%\pg_ctl.exe" -D "%PGDATA%" -w stop >nul 2>&1

echo.
echo ^✅ Parado. Os dados continuam salvos.
echo.
pause
```

- [ ] **Step 5: Escrever o `LEIA-ME.txt`**

`packaging/windows/LEIA-ME.txt`:

```
GoFacialEmulator - versao Windows (nao precisa de Docker)

O QUE FAZER
  1. Duplo-clique em INSTALAR.bat  (so na primeira vez, demora ~1 minuto)
  2. Duplo-clique em INICIAR.bat
  3. Abra http://localhost:7070 no navegador.
  Para parar: PARAR.bat

  Nao e preciso instalar mais nada: o banco de dados ja vem junto.

LOGS
  Aplicacao: sistema\logs\trace.log  (e trace.html, colorido)
  Banco:     sistema\logs\postgres.log
  Instalacao: sistema\logs\instalacao.log

O manual completo esta no arquivo MANUAL.md do projeto.
```

- [ ] **Step 6: Acrescentar o alvo `windows` ao `build-pacotes.bat`**

Em `packaging/build-pacotes.bat`, inserir o bloco abaixo **entre** o fim do bloco docker e o rótulo `:fim`. O batch executa rótulos em sequência: o bloco docker termina em `if /i not "%ALVO%"=="todos" goto fim`, então com `ALVO=todos` a execução cai naturalmente em `:build_windows`, e com `ALVO=docker` desvia para `:fim`.

```bat
REM ==================== WINDOWS ====================
:build_windows
set "PGCACHE=.build-cache\postgres-portable"
if not exist "%PGCACHE%\bin\postgres.exe" (
    echo [windows] Baixando o PostgreSQL portatil ^(uma vez^) ...
    if not exist .build-cache mkdir .build-cache
    powershell -NoProfile -Command ^
      "$u='https://get.enterprisedb.com/postgresql/postgresql-15.8-1-windows-x64-binaries.zip'; Invoke-WebRequest -Uri $u -OutFile '.build-cache\pg.zip'; Expand-Archive -Path '.build-cache\pg.zip' -DestinationPath '.build-cache\pg' -Force; Move-Item '.build-cache\pg\pgsql' '%PGCACHE%' -Force; Remove-Item '.build-cache\pg.zip','.build-cache\pg' -Recurse -Force"
    if not exist "%PGCACHE%\bin\postgres.exe" (
        echo [ERRO] Falha ao baixar o PostgreSQL portatil.
        exit /b 1
    )
)

set "STAGE=%OUT%\windows"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%\sistema\logs"
mkdir "%STAGE%\sistema\configs"

echo [windows] Compilando ...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o "%STAGE%\sistema\emulator-service.exe" cmd\emulator-service\main.go
if errorlevel 1 (
    echo [ERRO] Falha na compilacao.
    exit /b 1
)

echo [windows] Copiando o PostgreSQL portatil ...
xcopy /E /I /Y /Q "%PGCACHE%\bin"   "%STAGE%\sistema\postgres\bin"   >nul
xcopy /E /I /Y /Q "%PGCACHE%\lib"   "%STAGE%\sistema\postgres\lib"   >nul
xcopy /E /I /Y /Q "%PGCACHE%\share" "%STAGE%\sistema\postgres\share" >nul

copy /Y packaging\windows\config.yaml   "%STAGE%\sistema\configs\config.yaml" >nul
copy /Y packaging\windows\INSTALAR.bat  "%STAGE%\" >nul
copy /Y packaging\windows\INICIAR.bat   "%STAGE%\" >nul
copy /Y packaging\windows\PARAR.bat     "%STAGE%\" >nul
copy /Y packaging\windows\LEIA-ME.txt   "%STAGE%\" >nul

if exist "%OUT%\GoFacialEmulator-windows.zip" del "%OUT%\GoFacialEmulator-windows.zip"
powershell -NoProfile -Command "Compress-Archive -Path '%STAGE%\*' -DestinationPath '%OUT%\GoFacialEmulator-windows.zip' -Force"
if errorlevel 1 (
    echo [ERRO] Falha ao gerar o ZIP.
    exit /b 1
)
echo [windows] OK: %OUT%\GoFacialEmulator-windows.zip
if /i not "%ALVO%"=="todos" goto fim
```

E, no fim do bloco docker, quando o alvo for `todos`, cair em `:build_windows` (o fluxo do batch segue para o rótulo seguinte naturalmente — basta que `:build_windows` venha logo depois do bloco docker).

- [ ] **Step 7: Gerar o pacote**

Run: `packaging\build-pacotes.bat windows`
Expected: `[windows] OK: packaging\.out\GoFacialEmulator-windows.zip`, com o download do PostgreSQL acontecendo apenas na primeira execução.

Run: `packaging\build-pacotes.bat windows` (segunda vez)
Expected: mesma saída, sem baixar nada de novo.

- [ ] **Step 8: Testar o pacote de ponta a ponta**

Extrair o ZIP numa pasta vazia fora do repositório, numa máquina **sem** Docker, e nela:

Run: `INSTALAR.bat`
Expected: `✅ Instalado. Rode INICIAR.bat`

Run: `INSTALAR.bat` (segunda vez)
Expected: `O banco ja estava instalado. Nada a fazer.` seguido de `✅ Instalado.`

Run: `INICIAR.bat`
Expected: `✅ Rodando em http://localhost:7070`

Abrir http://localhost:7070 e confirmar que a página carrega com CSS (prova que o `go:embed` dos assets funciona sem a pasta `web/`).

Fechar a janela minimizada da aplicação pelo Gerenciador de Tarefas e rodar `PARAR.bat`.
Expected: `✅ Parado.` e nenhum processo `postgres.exe` sobrando (`tasklist | findstr postgres`).

Conferir que existem `sistema\logs\trace.log` e `sistema\logs\postgres.log`.

- [ ] **Step 9: Commit**

```bash
git add packaging/
git commit -m "$(cat <<'EOF'
feat(packaging): add Windows package with embedded PostgreSQL

O pacote portátil antigo mandava o usuário baixar os binários do
EnterpriseDB e copiar pastas na mão — bloqueio total para o perfil de
usuário alvo. Agora o build baixa os binários uma vez para .build-cache/
e embute tudo no ZIP.

PARAR.bat encerra o Postgres mesmo quando a janela da aplicação já foi
fechada, corrigindo o processo órfão do pacote anterior.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Pacote Linux, servindo também WSL2

**Files:**
- Create: `packaging/linux/instalar.sh`
- Create: `packaging/linux/iniciar.sh`
- Create: `packaging/linux/parar.sh`
- Create: `packaging/linux/LEIA-ME.txt`
- Create: `packaging/linux/config.yaml`
- Modify: `packaging/build-pacotes.bat`

**Interfaces:**
- Consumes: `packaging/build-pacotes.bat` das Tasks 4 e 5.
- Produces: `packaging/.out/GoFacialEmulator-linux.zip`. PID da aplicação em `sistema/logs/app.pid`.

- [ ] **Step 1: Escrever o `config.yaml` do pacote**

`packaging/linux/config.yaml` — idêntico ao `configs/config.yaml` da Task 2 (banco em `127.0.0.1:5432`). Copiar o conteúdo do Step 3 da Task 2 tal como está.

- [ ] **Step 2: Escrever o `instalar.sh`**

`packaging/linux/instalar.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log
: > "$LOG"

echo "=============================================================="
echo "  GoFacialEmulator - Instalacao (Linux)"
echo "=============================================================="

if [ "$(id -u)" -ne 0 ]; then
    echo
    echo "❌ Rode com sudo:  sudo ./instalar.sh"
    exit 1
fi

if grep -qiE "microsoft|wsl" /proc/version 2>/dev/null; then
    NO_WSL=1
    echo "      Ambiente detectado: WSL"
else
    NO_WSL=0
    echo "      Ambiente detectado: Linux"
fi

echo "[1/3] Instalando o PostgreSQL ..."
if ! command -v psql >/dev/null 2>&1; then
    apt-get update >>"$LOG" 2>&1
    if ! apt-get install -y postgresql postgresql-contrib >>"$LOG" 2>&1; then
        echo
        echo "❌ Falha ao instalar o PostgreSQL — veja sistema/logs/instalacao.log"
        exit 1
    fi
fi
echo "      PostgreSQL OK."

echo "[2/3] Ligando o banco ..."
if [ "$NO_WSL" -eq 1 ]; then
    service postgresql start >>"$LOG" 2>&1
else
    systemctl enable --now postgresql >>"$LOG" 2>&1
fi

for _ in $(seq 1 30); do
    if su - postgres -c "pg_isready" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

if ! su - postgres -c "pg_isready" >/dev/null 2>&1; then
    echo
    echo "❌ O banco nao ficou pronto — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Banco ligado."

echo "[3/3] Criando o usuario da aplicacao ..."
su - postgres -c "psql -tAc \"SELECT 1 FROM pg_roles WHERE rolname='emulator'\"" 2>>"$LOG" | grep -q 1 \
  || su - postgres -c "psql -c \"CREATE USER emulator WITH PASSWORD 'emulator123';\"" >>"$LOG" 2>&1

su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='emulator_db'\"" 2>>"$LOG" | grep -q 1 \
  || su - postgres -c "psql -c \"CREATE DATABASE emulator_db OWNER emulator;\"" >>"$LOG" 2>&1

if ! su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='emulator_db'\"" 2>>"$LOG" | grep -q 1; then
    echo
    echo "❌ Falha ao criar o banco emulator_db — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Usuario e banco OK."

chmod +x sistema/emulator-service iniciar.sh parar.sh

echo
echo "✅ Instalado. Rode ./iniciar.sh"
```

As duas criações são condicionais, o que torna o script idempotente.

- [ ] **Step 3: Escrever o `iniciar.sh`**

`packaging/linux/iniciar.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
PIDFILE=sistema/logs/app.pid

echo "=============================================================="
echo "  GoFacialEmulator - Iniciando"
echo "=============================================================="

if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
    echo "❌ Ja esta rodando. Abra http://localhost:7070 ou rode ./parar.sh antes."
    exit 1
fi

echo "[1/3] Ligando o banco ..."
if grep -qiE "microsoft|wsl" /proc/version 2>/dev/null; then
    sudo service postgresql start >/dev/null 2>&1
else
    sudo systemctl start postgresql >/dev/null 2>&1
fi

echo "[2/3] Iniciando a aplicacao ..."
cd sistema
nohup ./emulator-service -config configs/config.yaml >logs/app.out 2>&1 &
echo $! > logs/app.pid
cd ..

echo "[3/3] Aguardando a aplicacao responder ..."
for _ in $(seq 1 60); do
    if curl -sf http://localhost:7070/monitoring/health/quick >/dev/null 2>&1; then
        echo
        echo "✅ Rodando em http://localhost:7070"
        echo
        echo "   Para parar: ./parar.sh"
        exit 0
    fi
    sleep 1
done

echo
echo "❌ A aplicacao nao respondeu em 60 segundos."
echo "   Veja sistema/logs/trace.log e sistema/logs/app.out"
exit 1
```

- [ ] **Step 4: Escrever o `parar.sh`**

`packaging/linux/parar.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

PIDFILE=sistema/logs/app.pid

echo "Parando a aplicacao ..."
if [ -f "$PIDFILE" ]; then
    kill "$(cat "$PIDFILE")" 2>/dev/null
    rm -f "$PIDFILE"
else
    pkill -f "emulator-service -config" 2>/dev/null
fi

echo
echo "✅ Parado. Os dados continuam salvos."
echo "   O banco de dados continua ligado (é um serviço do sistema)."
```

- [ ] **Step 5: Escrever o `LEIA-ME.txt`**

`packaging/linux/LEIA-ME.txt`:

```
GoFacialEmulator - versao Linux (serve tambem para WSL2)

O QUE FAZER
  1. sudo ./instalar.sh    (so na primeira vez)
  2. ./iniciar.sh
  3. Abra http://localhost:7070 no navegador.
  Para parar: ./parar.sh

  No WSL2 os mesmos comandos valem: o instalador detecta o ambiente
  sozinho e usa "service" no lugar de "systemctl".

LOGS
  Aplicacao: sistema/logs/trace.log  (e trace.html, colorido)
  Saida bruta: sistema/logs/app.out
  Banco:     /var/log/postgresql/

O manual completo esta no arquivo MANUAL.md do projeto.
```

- [ ] **Step 6: Acrescentar o alvo `linux` ao `build-pacotes.bat`**

Inserir antes do rótulo `:fim`, logo após o bloco windows:

```bat
REM ==================== LINUX ====================
:build_linux
set "STAGE=%OUT%\linux"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%\sistema\logs"
mkdir "%STAGE%\sistema\configs"

echo [linux] Compilando ...
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o "%STAGE%\sistema\emulator-service" cmd\emulator-service\main.go
if errorlevel 1 (
    echo [ERRO] Falha na compilacao.
    exit /b 1
)

copy /Y packaging\linux\config.yaml  "%STAGE%\sistema\configs\config.yaml" >nul
copy /Y packaging\linux\instalar.sh  "%STAGE%\" >nul
copy /Y packaging\linux\iniciar.sh   "%STAGE%\" >nul
copy /Y packaging\linux\parar.sh     "%STAGE%\" >nul
copy /Y packaging\linux\LEIA-ME.txt  "%STAGE%\" >nul

if exist "%OUT%\GoFacialEmulator-linux.zip" del "%OUT%\GoFacialEmulator-linux.zip"
powershell -NoProfile -Command "Compress-Archive -Path '%STAGE%\*' -DestinationPath '%OUT%\GoFacialEmulator-linux.zip' -Force"
if errorlevel 1 (
    echo [ERRO] Falha ao gerar o ZIP.
    exit /b 1
)
echo [linux] OK: %OUT%\GoFacialEmulator-linux.zip
```

O `Compress-Archive` não preserva o bit de execução; por isso `instalar.sh` roda `chmod +x` nos outros dois scripts, e o `LEIA-ME.txt` instrui a começar por `sudo ./instalar.sh` (que o usuário invoca via `sudo bash instalar.sh` se o bit não vier). Documentar isso no `MANUAL.md` na Task 8.

- [ ] **Step 7: Gerar os três pacotes de uma vez**

Run: `packaging\build-pacotes.bat todos`
Expected: os três ZIPs listados ao final em `packaging\.out\`

- [ ] **Step 8: Testar em WSL2**

No WSL2, copiar o ZIP para uma pasta vazia e:

Run: `unzip GoFacialEmulator-linux.zip -d gofe && cd gofe && sudo bash instalar.sh`
Expected: `Ambiente detectado: WSL` e, ao final, `✅ Instalado. Rode ./iniciar.sh`

Run: `sudo bash instalar.sh` (segunda vez)
Expected: mesma saída de sucesso, sem erro de usuário/banco já existente

Run: `./iniciar.sh`
Expected: `✅ Rodando em http://localhost:7070`

Run: `curl -sf http://localhost:7070/monitoring/health/quick`
Expected: JSON com `status`

Run: `./parar.sh`
Expected: `✅ Parado.`

Conferir que `sistema/logs/trace.log` existe.

- [ ] **Step 9: Commit**

```bash
git add packaging/
git commit -m "$(cat <<'EOF'
feat(packaging): add Linux package serving WSL2 as well

Um script só para os dois ambientes: instalar.sh lê /proc/version e troca
systemctl por service quando está sob WSL. Criação de usuário e banco é
condicional, então rodar de novo não falha.

parar.sh usa o PID gravado em sistema/logs/app.pid, com pkill como reserva.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Alternar online/standalone pela interface

Item 2 do `new_tasks.md`. Os dois modos já existem e são controlados por `LocalAuthentication` em `emulator.device_settings` (`0` = online, o Site Controller valida; `1` = standalone, o dispositivo valida e gera o evento). Hoje só o Site Controller consegue trocar esse valor, escrevendo em `acsCfg`. Falta expor o toggle para poder testar sem o SC.

**Files:**
- Create: `internal/handlers/device_mode.go`
- Create: `internal/handlers/device_mode_test.go`
- Modify: `internal/handlers/handlers.go:235-238` (grupo de rotas `devices`)
- Modify: `internal/handlers/handlers.go:950-961` (mapa de dispositivo em `getCurrentDevicesWithFilters`)
- Modify: `assets/web/templates/devices.html:88-100` e `:106-134`
- Modify: `assets/web/static/js/main.js`

**Interfaces:**
- Consumes: `h.serviceDB database.DBInterface` (o `service_db` e o `emulator_db` apontam para o mesmo banco físico, conforme `config.Load`, que copia `ServiceDB` para `EmulatorDB`).
- Produces:
  - `func (h *Handler) getDeviceMode(ctx context.Context, deviceID int) (string, error)` — retorna `"online"` ou `"standalone"`.
  - `func (h *Handler) setDeviceMode(ctx context.Context, deviceID int, mode string) error`.
  - `GET /api/devices/:id/mode` → `{"mode":"online"}`.
  - `POST /api/devices/:id/mode` com corpo `{"mode":"standalone"}` → `{"mode":"standalone"}`.
  - Chave `local_auth` (string `"online"`/`"standalone"`) no mapa de cada dispositivo consumido por `devices.html`.

- [ ] **Step 1: Escrever os testes que falham**

Criar `internal/handlers/device_mode_test.go`. O duplo de banco implementa `database.DBInterface`; só `QueryRow` e `Exec` são exercitados.

```go
package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeRow struct {
	valor string
	err   error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	p, ok := dest[0].(*string)
	if !ok {
		return errors.New("destino nao e *string")
	}
	*p = r.valor
	return nil
}

type fakeDB struct {
	row       fakeRow
	execQuery string
	execArgs  []interface{}
	execErr   error
}

func (d *fakeDB) Query(ctx context.Context, q string, args ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("nao usado")
}

func (d *fakeDB) QueryRow(ctx context.Context, q string, args ...interface{}) pgx.Row {
	return d.row
}

func (d *fakeDB) Exec(ctx context.Context, q string, args ...interface{}) (pgconn.CommandTag, error) {
	d.execQuery = q
	d.execArgs = args
	return pgconn.CommandTag{}, d.execErr
}

func (d *fakeDB) Begin(ctx context.Context) (pgx.Tx, error) { return nil, errors.New("nao usado") }
func (d *fakeDB) Ping(ctx context.Context) error            { return nil }

func TestGetDeviceMode(t *testing.T) {
	casos := []struct {
		nome     string
		row      fakeRow
		esperado string
	}{
		{"valor 0 é online", fakeRow{valor: "0"}, "online"},
		{"valor 1 é standalone", fakeRow{valor: "1"}, "standalone"},
		{"sem registro cai no padrão standalone", fakeRow{err: pgx.ErrNoRows}, "standalone"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := &Handler{serviceDB: &fakeDB{row: c.row}}
			modo, err := h.getDeviceMode(context.Background(), 7)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if modo != c.esperado {
				t.Errorf("modo = %q, quero %q", modo, c.esperado)
			}
		})
	}
}

func TestSetDeviceMode(t *testing.T) {
	casos := []struct {
		modo     string
		esperado string
	}{
		{"online", "0"},
		{"standalone", "1"},
	}

	for _, c := range casos {
		t.Run(c.modo, func(t *testing.T) {
			db := &fakeDB{}
			h := &Handler{serviceDB: db}
			if err := h.setDeviceMode(context.Background(), 7, c.modo); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(db.execArgs) != 3 {
				t.Fatalf("esperava 3 argumentos, veio %d", len(db.execArgs))
			}
			if db.execArgs[0] != 7 {
				t.Errorf("device_id = %v, quero 7", db.execArgs[0])
			}
			if db.execArgs[1] != "LocalAuthentication" {
				t.Errorf("cfg_id = %v, quero LocalAuthentication", db.execArgs[1])
			}
			if db.execArgs[2] != c.esperado {
				t.Errorf("value = %v, quero %v", db.execArgs[2], c.esperado)
			}
		})
	}
}

func TestSetDeviceModeRejeitaValorInvalido(t *testing.T) {
	db := &fakeDB{}
	h := &Handler{serviceDB: db}
	err := h.setDeviceMode(context.Background(), 7, "turbo")
	if err == nil {
		t.Fatal("esperava erro para modo invalido")
	}
	if db.execQuery != "" {
		t.Error("nao deveria ter escrito no banco")
	}
}
```

- [ ] **Step 2: Rodar os testes e confirmar que falham**

Run: `go test ./internal/handlers/ -run TestGetDeviceMode -v`
Expected: FAIL na compilação — `h.getDeviceMode undefined`

- [ ] **Step 3: Implementar as funções**

Criar `internal/handlers/device_mode.go`:

```go
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Modos de operação do dispositivo, gravados em emulator.device_settings
// sob a chave LocalAuthentication:
//   "0" = online     — o Site Controller valida o acesso (remoteCheck)
//   "1" = standalone — o dispositivo valida localmente e gera o evento
const (
	modoOnline     = "online"
	modoStandalone = "standalone"

	chaveLocalAuth = "LocalAuthentication"
)

// getDeviceMode lê o modo de operação do dispositivo. Sem registro, o padrão
// é standalone — o mesmo padrão aplicado pelos repositórios dos emuladores.
func (h *Handler) getDeviceMode(ctx context.Context, deviceID int) (string, error) {
	var valor string
	err := h.serviceDB.QueryRow(ctx,
		"SELECT value FROM emulator.device_settings WHERE device_id = $1 AND cfg_id = $2",
		deviceID, chaveLocalAuth).Scan(&valor)
	if err != nil {
		return modoStandalone, nil
	}

	if valor == "0" {
		return modoOnline, nil
	}
	return modoStandalone, nil
}

// setDeviceMode grava o modo de operação do dispositivo.
func (h *Handler) setDeviceMode(ctx context.Context, deviceID int, mode string) error {
	var valor string
	switch mode {
	case modoOnline:
		valor = "0"
	case modoStandalone:
		valor = "1"
	default:
		return fmt.Errorf("modo invalido: %q (use %q ou %q)", mode, modoOnline, modoStandalone)
	}

	_, err := h.serviceDB.Exec(ctx,
		`INSERT INTO emulator.device_settings (device_id, cfg_id, value)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (device_id, cfg_id) DO UPDATE SET value = $3, updated_at = NOW()`,
		deviceID, chaveLocalAuth, valor)
	return err
}

// getDeviceModeHandler atende GET /api/devices/:id/mode
func (h *Handler) getDeviceModeHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id invalido"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	modo, err := h.getDeviceMode(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": modo})
}

// setDeviceModeHandler atende POST /api/devices/:id/mode
func (h *Handler) setDeviceModeHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id invalido"})
		return
	}

	var corpo struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&corpo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "corpo invalido"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := h.setDeviceMode(ctx, id, corpo.Mode); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.tracer.Info("Device %d mode set to %s", id, corpo.Mode)
	c.JSON(http.StatusOK, gin.H{"mode": corpo.Mode})
}
```

- [ ] **Step 4: Rodar os testes e confirmar que passam**

Run: `go test ./internal/handlers/ -v`
Expected: PASS

- [ ] **Step 5: Registrar as rotas**

Em `internal/handlers/handlers.go`, no grupo `devices` (logo após `devices.POST("/:id/stop", h.stopSingleDevice)`):

```go
			devices.GET("/:id/mode", h.getDeviceModeHandler)
			devices.POST("/:id/mode", h.setDeviceModeHandler)
```

- [ ] **Step 6: Expor o modo no mapa de dispositivos**

Em `internal/handlers/handlers.go`, dentro do laço que monta `currentDevices` (por volta da linha 950), acrescentar a leitura do modo. O laço já tem um `device` em mãos:

```go
		modo, _ := h.getDeviceMode(context.Background(), device.ID)

		currentDevices = append(currentDevices, map[string]interface{}{
			"lc_id":       device.ID,
			"name":        device.Name,
			"ip_address":  device.IPAddress,
			"port":        device.Port,
			"log_enabled": device.LogEnabled,
			"model":       device.Model,
			"status":      status,
			"enabled":     device.Enabled,
			"interval":    device.EventInterval,
			"total":       device.TotalUsers,
			"local_auth":  modo,
		})
```

- [ ] **Step 7: Acrescentar a coluna na tabela**

Em `assets/web/templates/devices.html`, no `<thead>`, depois da coluna `Port`:

```html
        <th scope="col" class="text-center">Modo</th>
```

E, no `<tbody>`, depois da célula `port_no`:

```html
          <td class="text-center">
            <select
              class="form-select form-select-sm"
              id="mode_{{ .lc_id }}"
              onchange="alterarModo('{{ .lc_id }}', this.value)"
              title="Online: o Site Controller valida o acesso. Standalone: o dispositivo valida sozinho."
            >
              <option value="online" {{ if eq .local_auth "online" }}selected{{ end }}>Online</option>
              <option value="standalone" {{ if eq .local_auth "standalone" }}selected{{ end }}>Standalone</option>
            </select>
          </td>
```

- [ ] **Step 8: Acrescentar a função JavaScript**

Ao final de `assets/web/static/js/main.js`:

```javascript
/**
 * Alterna o modo de operação do dispositivo entre online e standalone.
 * Online: o Site Controller valida o acesso.
 * Standalone: o dispositivo valida localmente e gera o evento.
 */
async function alterarModo(deviceId, modo) {
    try {
        const resposta = await fetch(`/api/devices/${deviceId}/mode`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mode: modo })
        });

        if (!resposta.ok) {
            const erro = await resposta.json();
            throw new Error(erro.error || 'falha ao alterar o modo');
        }

        showNotification(`Dispositivo ${deviceId} agora está em modo ${modo}.`, 'success');
    } catch (erro) {
        showNotification(`Não foi possível alterar o modo: ${erro.message}`, 'danger');
    }
}
```

- [ ] **Step 9: Verificar na aplicação rodando**

Subir a aplicação com um Postgres disponível e:

Run: `curl -s http://localhost:7070/api/devices/1/mode`
Expected: `{"mode":"standalone"}`

Run: `curl -s -X POST http://localhost:7070/api/devices/1/mode -H "Content-Type: application/json" -d "{\"mode\":\"online\"}"`
Expected: `{"mode":"online"}`

Run: `curl -s http://localhost:7070/api/devices/1/mode`
Expected: `{"mode":"online"}`

Run: `curl -s -X POST http://localhost:7070/api/devices/1/mode -H "Content-Type: application/json" -d "{\"mode\":\"turbo\"}"`
Expected: HTTP 400 com `modo invalido`

Abrir http://localhost:7070 e confirmar que a coluna "Modo" mostra o valor atual e que trocar no select exibe a notificação.

- [ ] **Step 10: Commit**

```bash
git add internal/handlers/device_mode.go internal/handlers/device_mode_test.go internal/handlers/handlers.go assets/web/templates/devices.html assets/web/static/js/main.js
git commit -m "$(cat <<'EOF'
feat(devices): toggle online/standalone mode from the UI

Os dois modos já existiam, controlados por LocalAuthentication em
emulator.device_settings, mas só o Site Controller conseguia trocá-los
escrevendo em acsCfg. Agora há coluna na tabela de dispositivos e
GET/POST /api/devices/:id/mode, para testar cada modo sem depender do SC.

O comportamento dos emuladores não muda.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Manual único e limpeza da documentação

Itens 4 e 5 do `new_tasks.md`.

**Files:**
- Create: `MANUAL.md`
- Modify: `README.md`
- Modify: `DOCKER.md`, `PORTAS-EMULADORES.md`
- Delete: `IMPROVEMENTS_SUMMARY.md`

**Interfaces:**
- Consumes: tudo das Tasks 1 a 7 — os nomes de script, caminhos de log e portas citados no manual precisam bater com o que foi construído.
- Produces: `MANUAL.md` como porta de entrada única.

- [ ] **Step 1: Escrever o `MANUAL.md`**

Criar `MANUAL.md` na raiz, com esta estrutura e conteúdo:

````markdown
# Manual do GoFacialEmulator

O GoFacialEmulator simula dispositivos de controle de acesso facial (Hikvision e
Dahua) para testar o W-Access sem hardware real.

## 1. Qual pacote usar

| Sua situação | Pacote |
|---|---|
| Windows com Docker Desktop instalado | `GoFacialEmulator-docker.zip` |
| Windows sem Docker | `GoFacialEmulator-windows.zip` |
| Servidor Linux ou WSL2 | `GoFacialEmulator-linux.zip` |

Na dúvida no Windows, use `GoFacialEmulator-windows.zip`: não depende de mais nada.

## 2. Instalação

### 2.1 Windows sem Docker

1. Extraia o ZIP em `C:\GoFacialEmulator` (evite Área de Trabalho, Documentos e
   OneDrive — a sincronização atrapalha o banco de dados).
2. Duplo-clique em `INSTALAR.bat`. Demora cerca de um minuto. Espere a mensagem
   `✅ Instalado`.
3. Duplo-clique em `INICIAR.bat`. Espere `✅ Rodando em http://localhost:7070`.
4. Abra http://localhost:7070 no navegador.

Para parar: `PARAR.bat`.

### 2.2 Windows com Docker Desktop

1. Abra o Docker Desktop e espere o ícone ficar verde.
2. Extraia o ZIP em `C:\GoFacialEmulator`.
3. `INSTALAR.bat` → `INICIAR.bat` → http://localhost:7070.

Para parar: `PARAR.bat`.

### 2.3 Linux ou WSL2

1. Extraia o ZIP em `/opt/gofacialemulator` (ou na home do usuário).
2. `sudo bash instalar.sh`
3. `./iniciar.sh`
4. Abra http://localhost:7070.

Para parar: `./parar.sh`.

O `instalar.sh` detecta WSL sozinho e ajusta o que muda entre WSL e Linux comum.
Se os scripts não estiverem executáveis (o ZIP não preserva essa permissão), use
`bash iniciar.sh` e `bash parar.sh`.

## 3. Primeiro uso

1. Abra http://localhost:7070/settings.
2. Preencha os dados do banco do W-Access: servidor, porta (1433), banco
   (`W_Access`), usuário e senha.
3. Clique em testar a conexão e salve.
4. Volte para http://localhost:7070 e clique em atualizar dispositivos: os
   controladores cujo campo descrição começa com `emulator` são carregados.
5. Clique em iniciar para subir os emuladores.

As portas dos emuladores vêm do W-Access, não da aplicação. Os pacotes publicam
o range 4000-4099; se um controlador estiver fora dele, ajuste a porta no
W-Access. Detalhes em [PORTAS-EMULADORES.md](PORTAS-EMULADORES.md).

### Modo online e standalone

Cada dispositivo tem uma coluna **Modo** na tabela:

- **Online** — o Site Controller valida o acesso e responde ao dispositivo.
- **Standalone** — o dispositivo valida localmente e gera o evento sozinho.

Trocar o valor no select vale imediatamente para o dispositivo.

## 4. Como rodar os testes

Os testes automatizados exigem o código-fonte e o Go instalado; não fazem parte
dos pacotes de instalação.

```bash
go test ./...
```

Verificação manual com o emulador rodando (troque 4001 pela porta do dispositivo):

```bash
# A aplicação está de pé?
curl http://localhost:7070/monitoring/health/quick

# O emulador responde como um dispositivo Hikvision?
curl http://localhost:4001/ISAPI/System/deviceInfo

# O stream de eventos abre?
curl -N http://localhost:4001/ISAPI/Event/notification/alertStream
```

## 5. Onde estão os logs

| Pacote | Aplicação | Banco de dados | Instalação |
|---|---|---|---|
| Windows | `sistema\logs\trace.log` e `trace.html` | `sistema\logs\postgres.log` | `sistema\logs\instalacao.log` |
| Docker | `sistema\logs\trace.log` | `docker compose -f sistema\docker-compose.yml logs postgres` | `sistema\logs\instalacao.log` |
| Linux / WSL | `sistema/logs/trace.log` e `app.out` | `/var/log/postgresql/` | `sistema/logs/instalacao.log` |

`trace.html` é o mesmo log em formato colorido: abra no navegador e pressione a
tecla `L` para filtrar por expressão.

Os logs giram sozinhos a cada 5 MB e são mantidos os 15 arquivos mais recentes.

## 6. Problemas comuns

**"A porta 7070 ja esta em uso"** — o emulador provavelmente já está rodando.
Abra http://localhost:7070. Se não for ele, rode `PARAR.bat` e tente de novo.

**"O Docker nao esta rodando"** — abra o Docker Desktop, espere o ícone ficar
verde e rode o script de novo.

**A aplicação não respondeu em 60 segundos** — veja `sistema/logs/trace.log`. A
causa mais comum é o banco não ter subido; no pacote Windows, confira
`sistema/logs/postgres.log`.

**A lista de dispositivos está vazia** — o banco do W-Access não foi configurado
em `/settings`, está inacessível, ou nenhum controlador local tem descrição
começando com `emulator`.

**A página abre sem formatação** — indica binário corrompido no download.
Baixe o ZIP de novo.

## Documentação técnica

- [DOCKER.md](DOCKER.md) — configuração avançada de Docker
- [PORTAS-EMULADORES.md](PORTAS-EMULADORES.md) — como as portas vêm do W-Access
- [MONITORING.md](MONITORING.md) — endpoints de métricas e health
- [README.md](README.md) — visão geral e arquitetura
````

- [ ] **Step 2: Enxugar o `README.md`**

Substituir a seção "🛠️ Instalação e Configuração" inteira (da linha 76 até o fim da Opção 2) por:

```markdown
## 🛠️ Instalação

Para instalar e usar o emulador, siga o **[MANUAL.md](MANUAL.md)**.

Para gerar os pacotes de instalação a partir do código:

```cmd
packaging\build-pacotes.bat todos
```

Os ZIPs saem em `packaging\.out\`.
```

Na seção "🔧 Configuração (.env)", corrigir a porta para 7070 e o banco para
`emulator_db`, e acrescentar a nota de que a configuração do W-Access é feita em
`/settings`. Nas seções de APIs, trocar `localhost:8080` por `localhost:7070`.

- [ ] **Step 3: Corrigir as portas nos documentos técnicos**

Em `DOCKER.md` e `PORTAS-EMULADORES.md`, substituir todas as ocorrências de
`8080` por `7070` e o range `4000-4999` por `4000-4099` (mantendo a menção de que
o range é ajustável). Em `DOCKER.md`, atualizar o mapeamento de volumes para
`./logs:/app/logs` apenas.

- [ ] **Step 4: Remover o documento morto**

```bash
rm -f IMPROVEMENTS_SUMMARY.md
```

É um relatório de dezembro/2025 sobre melhorias já incorporadas; não descreve o
estado atual nem serve de referência para o usuário.

- [ ] **Step 5: Verificar que nenhum link aponta para arquivo inexistente**

Run: `git grep -oh "\](\([A-Za-z0-9._/-]*\.md\))" -- "*.md" | tr -d '](' | sort -u | while read f; do [ -e "$f" ] || echo "QUEBRADO: $f"; done`
Expected: nenhuma saída

Run: `git grep -n "8080" -- "*.md"`
Expected: nenhuma saída

- [ ] **Step 6: Commit**

```bash
git add MANUAL.md README.md DOCKER.md PORTAS-EMULADORES.md IMPROVEMENTS_SUMMARY.md
git commit -m "$(cat <<'EOF'
docs: replace scattered guides with a single MANUAL.md

README-FIRST.md e README.md apontavam para quatro guias que nunca
existiram, e a porta citada variava entre 8080 e 7070. O MANUAL.md cobre
escolha de pacote, instalação por cenário, primeiro uso, como rodar os
testes, onde ficam os logs e problemas comuns.

DOCKER.md, PORTAS-EMULADORES.md e MONITORING.md passam a ser anexos
técnicos referenciados pelo manual.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Verificação final

Depois da Task 8, com os três ZIPs recém-gerados por `packaging\build-pacotes.bat todos`:

- [ ] `go build ./... && go test ./...` passa.
- [ ] Cada ZIP extraído numa pasta limpa instala e inicia usando só os scripts da raiz, sem passo manual.
- [ ] `GET http://localhost:7070/monitoring/health/quick` responde nos três pacotes.
- [ ] `INSTALAR` rodado duas vezes seguidas não falha nem apaga dados, nos três pacotes.
- [ ] `PARAR.bat` do pacote Windows encerra o Postgres mesmo após a janela da aplicação ter sido fechada.
- [ ] `logs/trace.log` existe e tem conteúdo após o primeiro start, nos três pacotes.
- [ ] `git grep -n "db_W-X-S@Wellcare924_\|172.16.17.67\|172.20.112.1"` não retorna nada.
- [ ] `git grep -n "8080" -- "*.md" docker-compose*.yml Dockerfile configs/` não retorna nada.
- [ ] Nenhum link `.md` aponta para arquivo inexistente.
