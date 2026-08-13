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
