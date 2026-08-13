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
set "SQLFILE=%TEMP%\gofe_instalar_criar.sql"
set "CHKUSER=%TEMP%\gofe_instalar_chkuser.txt"
set "CHKDB=%TEMP%\gofe_instalar_chkdb.txt"
del /f /q "%T1%" "%T2%" "%T3%" "%T4%" "%SQLFILE%" "%CHKUSER%" "%CHKDB%" >nul 2>&1

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

if exist "%PGDATA%\PG_VERSION" goto banco_existente

if not exist "%PGDATA%" goto criar_banco
set "PGDATA_VAZIA=1"
for /f %%F in ('dir /a /b "%PGDATA%" 2^>nul') do set "PGDATA_VAZIA=0"
if "%PGDATA_VAZIA%"=="0" (
    echo ^❌ A pasta sistema\postgres\data existe mas parece incompleta
    echo    ^(uma instalacao anterior deve ter sido interrompida^).
    echo    Apague a pasta sistema\postgres\data e rode INSTALAR.bat de novo.
    echo.
    pause
    exit /b 1
)

:criar_banco
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
goto ligar_banco

:banco_existente
echo O banco de dados ja existe, verificando se esta completo ...

:ligar_banco
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

REM So declarar "ja instalado" depois de confirmar que o usuario e o banco
REM da aplicacao realmente existem — o cluster pode ter sido criado numa
REM tentativa anterior que falhou logo em seguida, antes de cria-los.
set PGPASSWORD=postgres
"%PGBIN%\psql.exe" -h 127.0.0.1 -p %PGPORT% -U postgres -d postgres -t -A -c "SELECT 1 FROM pg_roles WHERE rolname='emulator';" >"%CHKUSER%" 2>nul
"%PGBIN%\psql.exe" -h 127.0.0.1 -p %PGPORT% -U postgres -d postgres -t -A -c "SELECT 1 FROM pg_database WHERE datname='emulator_db';" >"%CHKDB%" 2>nul
set "TEM_USUARIO="
set "TEM_BANCO="
set /p TEM_USUARIO=<"%CHKUSER%"
set /p TEM_BANCO=<"%CHKDB%"

if "%TEM_USUARIO%"=="1" if "%TEM_BANCO%"=="1" (
    "%PGBIN%\pg_ctl.exe" -D "%PGDATA%" -w stop >"%T4%" 2>&1
    type "%T1%" "%T2%" "%T4%" > "%LOG%" 2>nul
    del /f /q "%T1%" "%T2%" "%T3%" "%T4%" "%SQLFILE%" "%CHKUSER%" "%CHKDB%" >nul 2>&1
    set PGPASSWORD=
    echo O banco ja estava instalado. Nada a fazer.
    echo.
    echo ^✅ Instalado. Rode INICIAR.bat
    echo.
    pause
    exit /b 0
)

echo [3/3] Criando o usuario da aplicacao ...
(
echo DO $do$
echo BEGIN
echo    IF NOT EXISTS ^(SELECT FROM pg_catalog.pg_roles WHERE rolname = 'emulator'^) THEN
echo       CREATE ROLE emulator LOGIN PASSWORD 'emulator123';
echo    END IF;
echo END
echo $do$;
echo.
echo SELECT 'CREATE DATABASE emulator_db OWNER emulator'
echo WHERE NOT EXISTS ^(SELECT FROM pg_database WHERE datname = 'emulator_db'^)
echo \gexec
) > "%SQLFILE%"

"%PGBIN%\psql.exe" -h 127.0.0.1 -p %PGPORT% -U postgres -d postgres -v ON_ERROR_STOP=1 -f "%SQLFILE%" >"%T3%" 2>&1
set RC=%ERRORLEVEL%
set PGPASSWORD=

"%PGBIN%\pg_ctl.exe" -D "%PGDATA%" -w stop >"%T4%" 2>&1

type "%T1%" "%T2%" "%T3%" "%T4%" > "%LOG%" 2>nul
del /f /q "%T1%" "%T2%" "%T3%" "%T4%" "%SQLFILE%" "%CHKUSER%" "%CHKDB%" >nul 2>&1

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
