@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title GoFacialEmulator - Parando

if not exist sistema\logs mkdir sistema\logs
set "LOG=sistema\logs\instalacao.log"
set "T1=%TEMP%\gofe_docker_parar_1.log"
del /f /q "%T1%" >nul 2>&1

echo Parando o GoFacialEmulator ...
docker compose -f sistema\docker-compose.yml stop >"%T1%" 2>&1
if errorlevel 1 (
    type "%T1%" > "%LOG%" 2>nul
    echo.
    echo ^❌ Nao foi possivel parar — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)
type "%T1%" > "%LOG%" 2>nul
del /f /q "%T1%" >nul 2>&1

echo.
echo ^✅ Parado. Os dados continuam salvos.
echo.
pause
