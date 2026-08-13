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

tasklist /NH /FI "IMAGENAME eq postgres.exe" 2>nul | findstr /I "postgres.exe" >nul
if not errorlevel 1 (
    echo.
    echo ^❌ O banco nao parou por completo. Ainda ha processos postgres.exe rodando.
    echo    Feche-os pelo Gerenciador de Tarefas e rode PARAR.bat de novo.
    echo.
    pause
    exit /b 1
)

echo.
echo ^✅ Parado. Os dados continuam salvos.
echo.
pause
