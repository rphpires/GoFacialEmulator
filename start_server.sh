#!/bin/bash

echo "=== Iniciando GoFacialEmulator ==="

# Ler senha sudo do arquivo .env se existir
SUDO_PASSWORD=""
if [[ -f ".env" ]]; then
    SUDO_PASSWORD=$(grep "^SUDO_PASSWORD=" .env 2>/dev/null | cut -d'=' -f2- | tr -d "'" | tr -d '"')
fi

# Função para executar comando com sudo usando a senha
run_sudo() {
    if [[ -n "$SUDO_PASSWORD" ]]; then
        echo "$SUDO_PASSWORD" | sudo -S "$@"
    else
        sudo "$@"
    fi
}

# 1. Verificar PostgreSQL
echo "1. Verificando PostgreSQL..."
if ! run_sudo service postgresql status > /dev/null 2>&1; then
    echo "Iniciando PostgreSQL..."
    run_sudo service postgresql start
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
echo "URL Local:   http://localhost:7070"
echo "URL Windows: http://$WSL_IP:7070"
echo ""
echo "Para parar: Ctrl+C"
echo "Logs serão salvos em: logs/app.log"
echo ""

# 6. Iniciar aplicação
echo "Iniciando aplicação..."

# Obter o caminho completo do Go
GO_PATH=$(which go)
if [[ -z "$GO_PATH" ]]; then
    echo "❌ Erro: Go não encontrado no PATH"
    exit 1
fi

# Compilar primeiro
echo "Compilando aplicação..."
mkdir -p bin
if ! go build -o bin/emulator-service cmd/emulator-service/main.go; then
    echo "❌ Erro na compilação"
    exit 1
fi

# Executar o binário compilado com sudo
if [[ -n "$SUDO_PASSWORD" ]]; then
    echo "$SUDO_PASSWORD" | sudo -S ./bin/emulator-service 2>&1 | tee logs/app.log
else
    sudo ./bin/emulator-service 2>&1 | tee logs/app.log
fi