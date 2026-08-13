@echo off
setlocal enabledelayedexpansion
chcp 65001 >nul
cd /d "%~dp0.."

set "ALVO=%~1"
if "%ALVO%"=="" set "ALVO=todos"

set "OUT=packaging\.out"
if not exist "%OUT%" mkdir "%OUT%"

where go >nul 2>&1
if errorlevel 1 (
    echo [ERRO] Go nao encontrado. Instale em https://go.dev/dl/
    exit /b 1
)

if /i "%ALVO%"=="docker"  goto build_docker
if /i "%ALVO%"=="windows" goto build_windows
if /i "%ALVO%"=="linux"   goto build_linux
if /i "%ALVO%"=="todos"   goto build_docker
echo [ERRO] Alvo invalido: %ALVO%
echo Uso: build-pacotes.bat [docker^|windows^|linux^|todos]
exit /b 1

REM ==================== DOCKER ====================
:build_docker
echo.
echo [docker] Construindo a imagem ...
docker build -t gofacialemulator:1.0 .
if errorlevel 1 (
    echo [ERRO] docker build falhou.
    exit /b 1
)

set "STAGE=%OUT%\docker"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%\sistema\logs"

echo [docker] Exportando a imagem ...
docker save -o "%STAGE%\sistema\gofacialemulator-imagem.tar" gofacialemulator:1.0
if errorlevel 1 (
    echo [ERRO] docker save falhou.
    exit /b 1
)

copy /Y packaging\docker\docker-compose.yml "%STAGE%\sistema\" >nul
copy /Y packaging\docker\INSTALAR.bat  "%STAGE%\" >nul
copy /Y packaging\docker\INICIAR.bat   "%STAGE%\" >nul
copy /Y packaging\docker\PARAR.bat     "%STAGE%\" >nul
copy /Y packaging\docker\instalar.sh   "%STAGE%\" >nul
copy /Y packaging\docker\iniciar.sh    "%STAGE%\" >nul
copy /Y packaging\docker\parar.sh      "%STAGE%\" >nul
copy /Y packaging\docker\LEIA-ME.txt   "%STAGE%\" >nul

if exist "%OUT%\GoFacialEmulator-docker.zip" del "%OUT%\GoFacialEmulator-docker.zip"
powershell -NoProfile -Command "Compress-Archive -Path '%STAGE%\*' -DestinationPath '%OUT%\GoFacialEmulator-docker.zip' -Force"
if errorlevel 1 (
    echo [ERRO] Falha ao gerar o ZIP.
    exit /b 1
)
echo [docker] OK: %OUT%\GoFacialEmulator-docker.zip
if /i not "%ALVO%"=="todos" goto fim

REM ==================== WINDOWS ====================
:build_windows
set "PGCACHE=.build-cache\postgres-portable"
if not exist "%PGCACHE%\bin\postgres.exe" (
    echo [windows] Baixando o PostgreSQL portatil ^(uma vez^) ...
    if not exist .build-cache mkdir .build-cache
    powershell -NoProfile -Command ^
      "$u='https://get.enterprisedb.com/postgresql/postgresql-15.8-1-windows-x64-binaries.zip'; Invoke-WebRequest -Uri $u -OutFile '.build-cache\pg.zip'; Expand-Archive -Path '.build-cache\pg.zip' -DestinationPath '.build-cache\pg' -Force; Move-Item '.build-cache\pg\pgsql' '%PGCACHE%' -Force; Remove-Item '.build-cache\pg.zip','.build-cache\pg' -Recurse -Force"
    if not exist "%PGCACHE%\bin\postgres.exe" (
        echo [ERRO] Falha ao baixar o PostgreSQL portatil.
        exit /b 1
    )
)

set "STAGE=%OUT%\windows"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%\sistema\logs"
mkdir "%STAGE%\sistema\configs"

echo [windows] Compilando ...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o "%STAGE%\sistema\emulator-service.exe" cmd\emulator-service\main.go
if errorlevel 1 (
    echo [ERRO] Falha na compilacao.
    exit /b 1
)

echo [windows] Copiando o PostgreSQL portatil ...
xcopy /E /I /Y /Q "%PGCACHE%\bin"   "%STAGE%\sistema\postgres\bin"   >nul
xcopy /E /I /Y /Q "%PGCACHE%\lib"   "%STAGE%\sistema\postgres\lib"   >nul
xcopy /E /I /Y /Q "%PGCACHE%\share" "%STAGE%\sistema\postgres\share" >nul

copy /Y packaging\windows\config.yaml   "%STAGE%\sistema\configs\config.yaml" >nul
copy /Y packaging\windows\INSTALAR.bat  "%STAGE%\" >nul
copy /Y packaging\windows\INICIAR.bat   "%STAGE%\" >nul
copy /Y packaging\windows\PARAR.bat     "%STAGE%\" >nul
copy /Y packaging\windows\LEIA-ME.txt   "%STAGE%\" >nul

if exist "%OUT%\GoFacialEmulator-windows.zip" del "%OUT%\GoFacialEmulator-windows.zip"
powershell -NoProfile -Command "Compress-Archive -Path '%STAGE%\*' -DestinationPath '%OUT%\GoFacialEmulator-windows.zip' -Force"
if errorlevel 1 (
    echo [ERRO] Falha ao gerar o ZIP.
    exit /b 1
)
echo [windows] OK: %OUT%\GoFacialEmulator-windows.zip
if /i not "%ALVO%"=="todos" goto fim

:fim
echo.
echo Pacotes gerados em %OUT%\
dir /b "%OUT%\*.zip"
exit /b 0
