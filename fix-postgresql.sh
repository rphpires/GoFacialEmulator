#!/bin/bash

echo "=== Corrigindo PostgreSQL ==="

# 1. Parar todos os processos PostgreSQL
echo "1. Parando PostgreSQL..."
sudo systemctl stop postgresql
sudo pkill -f postgres 2>/dev/null

# 2. Verificar versão instalada
echo "2. Verificando versões instaladas..."
PG_VERSIONS=$(ls /etc/postgresql/ 2>/dev/null || echo "")
if [[ -z "$PG_VERSIONS" ]]; then
    echo "Nenhuma versão encontrada. Instalando PostgreSQL..."
    sudo apt update
    sudo apt install -y postgresql postgresql-contrib
fi

# 3. Descobrir versão principal
PG_VERSION=$(ls /etc/postgresql/ | head -1)
echo "Versão detectada: $PG_VERSION"

# 4. Iniciar serviço específico
echo "3. Iniciando PostgreSQL $PG_VERSION..."
if [[ -n "$PG_VERSION" ]]; then
    sudo systemctl start postgresql@$PG_VERSION-main
    sudo systemctl enable postgresql@$PG_VERSION-main
else
    sudo systemctl start postgresql
    sudo systemctl enable postgresql
fi

# 5. Aguardar inicialização
echo "4. Aguardando inicialização..."
sleep 3

# 6. Verificar se está rodando
echo "5. Verificando status..."
if ps aux | grep -v grep | grep postgres > /dev/null; then
    echo "✓ PostgreSQL está rodando"
    
    # Mostrar processos
    echo "Processos PostgreSQL:"
    ps aux | grep postgres | grep -v grep
    
    # Verificar porta
    if sudo netstat -tlnp | grep :5432 > /dev/null; then
        echo "✓ PostgreSQL escutando na porta 5432"
    else
        echo "✗ PostgreSQL não está escutando na porta 5432"
    fi
    
else
    echo "✗ PostgreSQL não está rodando"
    echo "Verificando logs..."
    sudo journalctl -u postgresql -n 10
    exit 1
fi

# 7. Testar conexão
echo "6. Testando conexão..."
if sudo -u postgres psql -c "SELECT version();" > /dev/null 2>&1; then
    echo "✓ Conexão com PostgreSQL OK"
    
    # Recriar usuário e bancos
    echo "7. Recriando usuário e bancos..."
    sudo -u postgres psql << 'EOF'
DROP ROLE IF EXISTS emulator;
CREATE ROLE emulator WITH LOGIN SUPERUSER PASSWORD 'emulator123';
DROP DATABASE IF EXISTS service_db;
DROP DATABASE IF EXISTS emulator_db;  
DROP DATABASE IF EXISTS wxs_db;
CREATE DATABASE service_db OWNER emulator;
CREATE DATABASE emulator_db OWNER emulator;
CREATE DATABASE wxs_db OWNER emulator;
\l
EOF
    
    echo "✓ Usuário e bancos criados"
    
else
    echo "✗ Não foi possível conectar ao PostgreSQL"
    exit 1
fi

echo ""
echo "=== PostgreSQL configurado com sucesso ==="
echo "Teste a aplicação agora com:"
echo "go run cmd/emulator-service/main.go"