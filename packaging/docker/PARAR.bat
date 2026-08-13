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
