@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title GoFacialEmulator

if not exist sistema\logs mkdir sistema\logs
set "LOG=sistema\logs\instalacao.log"

REM O comando grava num arquivo temporario proprio (nunca reaberto) para nao
REM disputar o mesmo arquivo com o antivirus; ao final e copiado para
REM sistema\logs\instalacao.log numa unica escrita.
set "T1=%TEMP%\gofe_docker_iniciar_1.log"
del /f /q "%T1%" >nul 2>&1

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
docker compose -f sistema\docker-compose.yml up -d >"%T1%" 2>&1
if errorlevel 1 (
    type "%T1%" > "%LOG%" 2>nul
    echo ^❌ Falha ao subir os servicos — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)
type "%T1%" > "%LOG%" 2>nul
del /f /q "%T1%" >nul 2>&1

echo [2/2] Aguardando a aplicacao responder ...
set /a tentativas=0
:esperar
set /a tentativas+=1
curl -s http://localhost:7070/monitoring/health/quick >nul 2>&1
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
