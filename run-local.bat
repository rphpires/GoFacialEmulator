@echo off
echo ========================================
echo  GoFacialEmulator - Desenvolvimento Local
echo ========================================
echo.

REM Verificar se o banco está rodando
docker ps | findstr facial-emulator-db >nul
if %errorlevel% neq 0 (
    echo [AVISO] Banco de dados nao esta rodando!
    echo Iniciando PostgreSQL...
    docker-compose -f docker-compose.db-only.yml up -d
    echo Aguardando banco inicializar...
    timeout /t 5 /nobreak >nul
)

echo [OK] Banco de dados rodando
echo.
echo Iniciando aplicacao localmente...
echo Usando config: configs/config.local.yaml
echo.

REM Executar aplicação com config local
go run cmd/emulator-service/main.go -config=configs/config.local.yaml

pause
