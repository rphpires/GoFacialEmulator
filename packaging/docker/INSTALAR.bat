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
