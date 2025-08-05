#!/bin/bash

echo "=== Iniciando GoFacialEmulator ==="

# 1. Verificar PostgreSQL
echo "1. Verificando PostgreSQL..."
if ! sudo service postgresql status > /dev/null 2>&1; then
    echo "Iniciando PostgreSQL..."
    sudo service postgresql start
    sleep 2
fi

# 2. Testar conexão com banco
echo "2. Testando conexão com banco..."
if ! PGPASSWORD=emulator123 psql -h localhost -U emulator -d service_db -c "SELECT 1;" > /dev/null 2>&1; then
    echo "❌ Erro: Não foi possível conectar ao banco de dados"
    echo "Verifique se PostgreSQL está configurado corretamente"
    exit 1
fi

echo "✅ Conexão com banco OK"

# 3. Verificar se está no diretório correto
if [[ ! -f "cmd/emulator-service/main.go" ]]; then
    echo "❌ Erro: Execute este script do diretório raiz do projeto"
    exit 1
fi

# 4. Criar diretórios necessários
mkdir -p logs traces

# 5. Mostrar informações
WSL_IP=$(ip addr show eth0 | grep "inet " | awk '{print $2}' | cut -d/ -f1)
echo ""
echo "=== Aplicação iniciando ==="
echo "URL Local:   http://localhost:8080"
echo "URL Windows: http://$WSL_IP:8080"
echo ""
echo "Para parar: Ctrl+C"
echo "Logs serão salvos em: logs/app.log"
echo ""

# 6. Iniciar aplicação
echo "Iniciando aplicação..."
go run cmd/emulator-service/main.go 2>&1 | tee logs/app.log