@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
title GoFacialEmulator - Instalacao

if not exist sistema\logs mkdir sistema\logs
set "LOG=sistema\logs\instalacao.log"

REM Cada comando grava num arquivo temporario proprio (nunca reaberto) para
REM nao disputar o mesmo arquivo em sequencia com o antivirus. No final,
REM tudo e reunido em sistema\logs\instalacao.log numa unica escrita.
set "T1=%TEMP%\gofe_docker_instalar_1.log"
set "T2=%TEMP%\gofe_docker_instalar_2.log"
set "T3=%TEMP%\gofe_docker_instalar_3.log"
del /f /q "%T1%" "%T2%" "%T3%" >nul 2>&1

echo ==============================================================
echo   GoFacialEmulator - Instalacao (Docker)
echo ==============================================================
echo.

echo [1/3] Verificando o Docker ...
docker info >"%T1%" 2>&1
if errorlevel 1 (
    type "%T1%" > "%LOG%" 2>nul
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
docker load -i sistema\gofacialemulator-imagem.tar >"%T2%" 2>&1
if errorlevel 1 (
    type "%T1%" "%T2%" > "%LOG%" 2>nul
    echo.
    echo ^❌ Falha ao carregar a aplicacao — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)
echo       Aplicacao carregada.

echo [3/3] Preparando o banco de dados ...
docker compose -f sistema\docker-compose.yml up -d --wait --wait-timeout 120 postgres >"%T3%" 2>&1
if errorlevel 1 (
    type "%T1%" "%T2%" "%T3%" > "%LOG%" 2>nul
    echo.
    echo ^❌ Falha ao preparar o banco — veja sistema\logs\instalacao.log
    echo.
    pause
    exit /b 1
)
echo       Banco preparado.

type "%T1%" "%T2%" "%T3%" > "%LOG%" 2>nul
del /f /q "%T1%" "%T2%" "%T3%" >nul 2>&1

echo.
echo ^✅ Instalado. Rode INICIAR.bat
echo.
pause
