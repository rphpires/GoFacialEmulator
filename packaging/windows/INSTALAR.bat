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

REM Cada etapa grava num arquivo temporario proprio (nunca reaberto) para
REM nao disputar o mesmo arquivo em sequencia com o antivirus. No final,
REM tudo e reunido em sistema\logs\instalacao.log numa unica escrita.
set "T1=%TEMP%\gofe_instalar_1.log"
set "T2=%TEMP%\gofe_instalar_2.log"
set "T3=%TEMP%\gofe_instalar_3.log"
set "T4=%TEMP%\gofe_instalar_4.log"
del /f /q "%T1%" "%T2%" "%T3%" "%T4%" >nul 2>&1

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
"%PGBIN%\initdb.exe" -D "%PGDATA%" -U postgres --pwfile="%PWFILE%" -E UTF8 --locale=C >"%T1%" 2>&1
set RC=%ERRORLEVEL%
del /f /q "%PWFILE%" >nul 2>&1
if not %RC%==0 (
    type "%T1%" > "%LOG%" 2>nul
    echo.
    echo ^❌ Falha ao criar o banco — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)

powershell -NoProfile -Command "(Get-Content '%PGDATA%\postgresql.conf') -replace '^#?port\s*=.*', 'port = %PGPORT%' | Set-Content '%PGDATA%\postgresql.conf'"

echo [2/3] Ligando o banco ...
"%PGBIN%\pg_ctl.exe" -D "%PGDATA%" -l "%~dp0sistema\logs\postgres.log" -w start >"%T2%" 2>&1
if errorlevel 1 (
    type "%T1%" "%T2%" > "%LOG%" 2>nul
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
  -c "CREATE DATABASE emulator_db OWNER emulator;" >"%T3%" 2>&1
set RC=%ERRORLEVEL%
set PGPASSWORD=

"%PGBIN%\pg_ctl.exe" -D "%PGDATA%" -w stop >"%T4%" 2>&1

type "%T1%" "%T2%" "%T3%" "%T4%" > "%LOG%" 2>nul
del /f /q "%T1%" "%T2%" "%T3%" "%T4%" >nul 2>&1

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
