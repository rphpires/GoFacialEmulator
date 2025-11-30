@echo off
setlocal enabledelayedexpansion

REM ============================================================
REM CONFIGURACAO - Edite aqui
REM ============================================================

REM Modo: docker, native ou auto
REM - docker: Usa docker-compose (PostgreSQL + Aplicacao em containers)
REM - native: Usa PostgreSQL instalado no Windows + compila aplicacao
REM - auto: Detecta automaticamente
set DEPLOY_MODE=auto

REM Credenciais
set DB_HOST=localhost
set DB_PORT=5432
set DB_USER=emulator
set DB_PASSWORD=emulator123
set DB_SERVICE=service_db
set DB_EMULATOR=emulator_db

REM Para modo native
set POSTGRES_SUPERUSER=postgres

REM ============================================================

cls
echo ============================================================
echo GoFacialEmulator - Deploy Automatico
echo ============================================================
echo.

REM Detectar modo
if "%DEPLOY_MODE%"=="auto" (
    where docker >nul 2>&1
    if !errorlevel!==0 (
        where docker-compose >nul 2>&1
        if !errorlevel!==0 (
            set DEPLOY_MODE=docker
            echo [*] Modo AUTO: Docker detectado, usando modo Docker
        ) else (
            where psql >nul 2>&1
            if !errorlevel!==0 (
                set DEPLOY_MODE=native
                echo [*] Modo AUTO: PostgreSQL detectado, usando modo Native
            ) else (
                set DEPLOY_MODE=native
                echo [*] Modo AUTO: Nada detectado, usando modo Native
            )
        )
    ) else (
        where psql >nul 2>&1
        if !errorlevel!==0 (
            set DEPLOY_MODE=native
            echo [*] Modo AUTO: PostgreSQL detectado, usando modo Native
        ) else (
            set DEPLOY_MODE=native
            echo [*] Modo AUTO: Nada detectado, usando modo Native
        )
    )
)

echo Modo: %DEPLOY_MODE%
echo.

REM ============================================================
REM MODO DOCKER (docker-compose)
REM ============================================================
if "%DEPLOY_MODE%"=="docker" (
    echo [*] Usando modo DOCKER ^(docker-compose^)
    echo.

    REM Verificar Docker
    where docker >nul 2>&1
    if errorlevel 1 (
        echo [!] Docker nao encontrado!
        echo [!] Instale Docker: https://docs.docker.com/desktop/windows/install/
        pause
        exit /b 1
    )
    echo [OK] Docker encontrado

    REM Verificar Docker Compose
    where docker-compose >nul 2>&1
    if errorlevel 1 (
        echo [!] Docker Compose nao encontrado!
        echo [!] Instale Docker Compose ou use Docker Desktop
        pause
        exit /b 1
    )
    echo [OK] Docker Compose encontrado

    REM Verificar docker-compose.yml
    if not exist docker-compose.yml (
        echo [!] Arquivo docker-compose.yml nao encontrado!
        pause
        exit /b 1
    )

    REM Parar containers antigos se existirem
    echo [*] Verificando containers existentes...
    docker ps -a | findstr "facial-emulator" >nul 2>&1
    if !errorlevel!==0 (
        echo [*] Parando containers antigos...
        docker-compose down 2>nul
        docker stop facial-emulator-postgres 2>nul
        docker rm facial-emulator-postgres 2>nul
        docker stop facial-emulator-db 2>nul
        docker rm facial-emulator-db 2>nul
        docker stop facial-emulator-app 2>nul
        docker rm facial-emulator-app 2>nul
        echo [OK] Containers antigos removidos
    )

    REM Construir e iniciar containers
    echo [*] Construindo e iniciando containers...
    docker-compose up -d --build

    REM Aguardar PostgreSQL
    echo [*] Aguardando PostgreSQL inicializar...
    timeout /t 5 /nobreak >nul

    :wait_docker_db
    docker exec facial-emulator-db pg_isready -U emulator >nul 2>&1
    if errorlevel 1 (
        timeout /t 1 /nobreak >nul
        goto wait_docker_db
    )
    echo [OK] PostgreSQL pronto

    REM Verificar bancos
    echo [*] Verificando bancos de dados...

    docker exec facial-emulator-db psql -U emulator -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='%DB_SERVICE%';" >nul 2>&1
    if errorlevel 1 (
        docker exec facial-emulator-db psql -U emulator -d postgres -c "CREATE DATABASE %DB_SERVICE% OWNER emulator;" >nul 2>&1
        docker exec facial-emulator-db psql -U emulator -d %DB_SERVICE% -c "CREATE SCHEMA IF NOT EXISTS service AUTHORIZATION emulator;" >nul 2>&1
        echo [OK] Banco '%DB_SERVICE%' criado
    ) else (
        echo [OK] Banco '%DB_SERVICE%' ja existe
    )

    docker exec facial-emulator-db psql -U emulator -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='%DB_EMULATOR%';" >nul 2>&1
    if errorlevel 1 (
        docker exec facial-emulator-db psql -U emulator -d postgres -c "CREATE DATABASE %DB_EMULATOR% OWNER emulator;" >nul 2>&1
        docker exec facial-emulator-db psql -U emulator -d %DB_EMULATOR% -c "CREATE SCHEMA IF NOT EXISTS emulator AUTHORIZATION emulator;" >nul 2>&1
        echo [OK] Banco '%DB_EMULATOR%' criado
    ) else (
        echo [OK] Banco '%DB_EMULATOR%' ja existe
    )

    REM Status dos containers
    echo.
    echo [*] Status dos containers:
    docker-compose ps

    goto summary

REM ============================================================
REM MODO NATIVE
REM ============================================================
) else if "%DEPLOY_MODE%"=="native" (
    echo [*] Usando modo NATIVE
    echo.

    REM Verificar PostgreSQL
    where psql >nul 2>&1
    if errorlevel 1 (
        echo [!] PostgreSQL nao encontrado!
        echo [!] Instale: https://www.postgresql.org/download/windows/
        echo [!] Adicione PostgreSQL ao PATH
        pause
        exit /b 1
    )
    echo [OK] PostgreSQL instalado

    REM Verificar servico
    sc query postgresql-x64-15 | find "RUNNING" >nul 2>&1
    if errorlevel 1 (
        sc query postgresql-x64-14 | find "RUNNING" >nul 2>&1
        if errorlevel 1 (
            echo [*] Iniciando PostgreSQL...
            net start postgresql-x64-15 >nul 2>&1
            if errorlevel 1 net start postgresql-x64-14 >nul 2>&1
            timeout /t 3 /nobreak >nul
        )
    )
    echo [OK] PostgreSQL rodando

    REM Criar usuario e bancos
    set TEMP_SQL=%TEMP%\setup_%RANDOM%.sql
    (
        echo DO $$ BEGIN
        echo IF EXISTS ^(SELECT 1 FROM pg_roles WHERE rolname='%DB_USER%'^) THEN
        echo   ALTER ROLE %DB_USER% WITH PASSWORD '%DB_PASSWORD%';
        echo ELSE
        echo   CREATE ROLE %DB_USER% WITH LOGIN PASSWORD '%DB_PASSWORD%';
        echo END IF;
        echo END $$;
        echo DROP DATABASE IF EXISTS %DB_SERVICE%;
        echo CREATE DATABASE %DB_SERVICE% OWNER %DB_USER%;
        echo DROP DATABASE IF EXISTS %DB_EMULATOR%;
        echo CREATE DATABASE %DB_EMULATOR% OWNER %DB_USER%;
    ) > "%TEMP_SQL%"

    set PGPASSWORD=%DB_PASSWORD%
    psql -U %POSTGRES_SUPERUSER% -h %DB_HOST% -p %DB_PORT% -f "%TEMP_SQL%" >nul 2>&1
    if errorlevel 1 (
        echo [!] Erro ao criar usuario e bancos
        del "%TEMP_SQL%" >nul 2>&1
        pause
        exit /b 1
    )
    echo [OK] Usuario e bancos criados
    del "%TEMP_SQL%" >nul 2>&1

    REM Criar schemas
    echo [*] Criando schemas...
    set TEMP_SQL=%TEMP%\schema_%RANDOM%.sql
    (
        echo CREATE SCHEMA IF NOT EXISTS service AUTHORIZATION %DB_USER%;
        echo GRANT ALL PRIVILEGES ON SCHEMA service TO %DB_USER%;
        echo ALTER DEFAULT PRIVILEGES IN SCHEMA service GRANT ALL ON TABLES TO %DB_USER%;
        echo ALTER DEFAULT PRIVILEGES IN SCHEMA service GRANT ALL ON SEQUENCES TO %DB_USER%;
    ) > "%TEMP_SQL%"
    psql -U %DB_USER% -h %DB_HOST% -p %DB_PORT% -d %DB_SERVICE% -f "%TEMP_SQL%" >nul 2>&1
    del "%TEMP_SQL%" >nul 2>&1

    set TEMP_SQL=%TEMP%\schema_%RANDOM%.sql
    (
        echo CREATE SCHEMA IF NOT EXISTS emulator AUTHORIZATION %DB_USER%;
        echo GRANT ALL PRIVILEGES ON SCHEMA emulator TO %DB_USER%;
        echo ALTER DEFAULT PRIVILEGES IN SCHEMA emulator GRANT ALL ON TABLES TO %DB_USER%;
        echo ALTER DEFAULT PRIVILEGES IN SCHEMA emulator GRANT ALL ON SEQUENCES TO %DB_USER%;
    ) > "%TEMP_SQL%"
    psql -U %DB_USER% -h %DB_HOST% -p %DB_PORT% -d %DB_EMULATOR% -f "%TEMP_SQL%" >nul 2>&1
    del "%TEMP_SQL%" >nul 2>&1
    echo [OK] Schemas configurados

    REM Criar .env
    if not exist .env (
        echo [*] Criando arquivo .env...
        (
            echo SERVER_HOST=0.0.0.0
            echo SERVER_PORT=8080
            echo.
            echo PG_HOST=%DB_HOST%
            echo PG_PORT=%DB_PORT%
            echo PG_USER=%DB_USER%
            echo PG_PASSWORD=%DB_PASSWORD%
            echo PG_DATABASE=%DB_SERVICE%
            echo.
            echo DB_HOST=%DB_HOST%
            echo DB_PORT=%DB_PORT%
            echo DB_USER=%DB_USER%
            echo DB_PASSWORD=%DB_PASSWORD%
            echo DB_DATABASE=%DB_EMULATOR%
            echo.
            echo WXS_HOST=172.20.112.1
            echo WXS_PORT=1433
            echo WXS_DATABASE=W_Access
            echo WXS_USER=W-Access
            echo WXS_PASSWORD=sua_senha_wxs
        ) > .env
        echo [OK] Arquivo .env criado
    ) else (
        echo [*] Arquivo .env ja existe
    )

    REM Compilar
    where go >nul 2>&1
    if not errorlevel 1 (
        echo [*] Compilando aplicacao...
        go mod download >nul 2>&1
        if not exist bin mkdir bin
        go build -o bin\emulator-service.exe cmd\emulator-service\main.go >nul 2>&1
        if not errorlevel 1 (
            echo [OK] Aplicacao compilada: bin\emulator-service.exe
        ) else (
            echo [!] Erro na compilacao
        )
    ) else (
        echo [*] Go nao encontrado
    )

    goto summary
) else (
    echo [!] Modo invalido: %DEPLOY_MODE%
    pause
    exit /b 1
)

REM ============================================================
REM RESUMO
REM ============================================================
:summary

echo.
echo ============================================================
echo Deploy Concluido!
echo ============================================================
echo.
echo [OK] Modo: %DEPLOY_MODE%

if "%DEPLOY_MODE%"=="docker" (
    echo [OK] Containers Docker criados e iniciados
    echo [OK] PostgreSQL: facial-emulator-db
    echo [OK] Aplicacao: facial-emulator-app
    echo.
    echo Comandos uteis:
    echo   docker-compose logs -f app
    echo   docker-compose logs -f postgres
    echo   docker-compose down
    echo   docker-compose restart
    echo   docker-compose ps
) else (
    echo [OK] PostgreSQL configurado
    echo [OK] Bancos criados: %DB_SERVICE%, %DB_EMULATOR%
    echo [OK] Schemas: service, emulator
    echo [OK] Usuario: %DB_USER%
    echo [OK] Arquivo .env configurado
    echo.
    echo Para iniciar:
    echo   bin\emulator-service.exe
    echo   ou
    echo   go run cmd\emulator-service\main.go
)

echo.
echo Acesse a aplicacao: http://localhost:8080
echo.
pause
