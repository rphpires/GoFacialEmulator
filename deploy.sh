#!/bin/bash

# ============================================================
# CONFIGURAÇÃO - Edite aqui
# ============================================================

# Modo de deploy: "docker" ou "native"
# - docker: Usa docker-compose (PostgreSQL + Aplicação em containers)
# - native: Usa PostgreSQL instalado no sistema + compila aplicação
# - auto: Detecta automaticamente (prefere Docker se disponível)
DEPLOY_MODE="${DEPLOY_MODE:-auto}"

# Credenciais do banco de dados (usadas em modo native e no docker-compose)
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-emulator}"
DB_PASSWORD="${DB_PASSWORD:-emulator123}"
DB_SERVICE="${DB_SERVICE:-service_db}"
DB_EMULATOR="${DB_EMULATOR:-emulator_db}"

# Para modo native: usuário superuser do PostgreSQL
POSTGRES_SUPERUSER="${POSTGRES_SUPERUSER:-postgres}"

# ============================================================

set -e
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_error() { echo -e "${RED}✗${NC} $1"; }
print_info() { echo -e "${BLUE}➜${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }

clear
echo -e "${BLUE}============================================================${NC}"
echo -e "${BLUE}GoFacialEmulator - Deploy Automático${NC}"
echo -e "${BLUE}============================================================${NC}"
echo ""

# Detectar modo automaticamente
if [[ "$DEPLOY_MODE" == "auto" ]]; then
    if command -v docker &> /dev/null && command -v docker-compose &> /dev/null; then
        DEPLOY_MODE="docker"
        print_info "Modo AUTO: Docker detectado, usando modo Docker"
    elif command -v psql &> /dev/null; then
        DEPLOY_MODE="native"
        print_info "Modo AUTO: PostgreSQL nativo detectado, usando modo Native"
    else
        print_warning "Modo AUTO: Nada detectado, tentando instalar PostgreSQL nativo..."
        DEPLOY_MODE="native"
    fi
fi

echo "Modo: $DEPLOY_MODE"
echo ""

# ============================================================
# MODO DOCKER (docker-compose)
# ============================================================
if [[ "$DEPLOY_MODE" == "docker" ]]; then
    print_info "Usando modo DOCKER (docker-compose)"
    echo ""

    # Verificar Docker e Docker Compose
    if ! command -v docker &> /dev/null; then
        print_error "Docker não está instalado!"
        print_info "Instale Docker: https://docs.docker.com/get-docker/"
        exit 1
    fi
    print_success "Docker encontrado: $(docker --version)"

    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose não está instalado!"
        print_info "Instale Docker Compose: https://docs.docker.com/compose/install/"
        exit 1
    fi
    print_success "Docker Compose encontrado: $(docker-compose --version)"

    # Verificar se docker-compose.yml existe
    if [[ ! -f docker-compose.yml ]]; then
        print_error "Arquivo docker-compose.yml não encontrado!"
        exit 1
    fi

    # Parar containers antigos se existirem
    print_info "Verificando containers existentes..."
    if docker ps -a | grep -q "facial-emulator"; then
        print_info "Parando containers antigos..."
        docker-compose down 2>/dev/null || true
        docker stop facial-emulator-postgres 2>/dev/null || true
        docker rm facial-emulator-postgres 2>/dev/null || true
        docker stop facial-emulator-db 2>/dev/null || true
        docker rm facial-emulator-db 2>/dev/null || true
        docker stop facial-emulator-app 2>/dev/null || true
        docker rm facial-emulator-app 2>/dev/null || true
        print_success "Containers antigos removidos"
    fi

    # Construir e iniciar containers
    print_info "Construindo e iniciando containers..."
    docker-compose up -d --build

    # Aguardar PostgreSQL inicializar
    print_info "Aguardando PostgreSQL inicializar..."
    sleep 5

    for i in {1..30}; do
        if docker exec facial-emulator-db pg_isready -U emulator > /dev/null 2>&1; then
            break
        fi
        sleep 1
    done
    print_success "PostgreSQL pronto"

    # Criar bancos (o docker-init.sql já faz isso, mas garantir)
    print_info "Verificando bancos de dados..."

    # Verificar se bancos existem
    DB_EXISTS=$(docker exec facial-emulator-db psql -U emulator -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_SERVICE';" 2>/dev/null || echo "0")
    if [[ "$DB_EXISTS" != "1" ]]; then
        docker exec facial-emulator-db psql -U emulator -d postgres -c "CREATE DATABASE $DB_SERVICE OWNER emulator;"
        docker exec facial-emulator-db psql -U emulator -d $DB_SERVICE -c "CREATE SCHEMA IF NOT EXISTS service AUTHORIZATION emulator;"
        print_success "Banco '$DB_SERVICE' criado"
    else
        print_success "Banco '$DB_SERVICE' já existe"
    fi

    DB_EXISTS=$(docker exec facial-emulator-db psql -U emulator -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_EMULATOR';" 2>/dev/null || echo "0")
    if [[ "$DB_EXISTS" != "1" ]]; then
        docker exec facial-emulator-db psql -U emulator -d postgres -c "CREATE DATABASE $DB_EMULATOR OWNER emulator;"
        docker exec facial-emulator-db psql -U emulator -d $DB_EMULATOR -c "CREATE SCHEMA IF NOT EXISTS emulator AUTHORIZATION emulator;"
        print_success "Banco '$DB_EMULATOR' criado"
    else
        print_success "Banco '$DB_EMULATOR' já existe"
    fi

    # Verificar status dos containers
    echo ""
    print_info "Status dos containers:"
    docker-compose ps

# ============================================================
# MODO NATIVE
# ============================================================
elif [[ "$DEPLOY_MODE" == "native" ]]; then
    print_info "Usando modo NATIVE (PostgreSQL instalado no sistema)"
    echo ""

    # Verificar e instalar PostgreSQL
    print_info "Verificando PostgreSQL..."
    if ! command -v psql &> /dev/null; then
        print_warning "PostgreSQL não instalado. Instalando..."
        if [[ -f /etc/debian_version ]]; then
            sudo apt update && sudo apt install -y postgresql postgresql-contrib
        elif [[ -f /etc/redhat-release ]]; then
            sudo yum install -y postgresql-server postgresql-contrib
            sudo postgresql-setup initdb
        else
            print_error "SO não suportado. Instale PostgreSQL manualmente."
            exit 1
        fi
    fi
    print_success "PostgreSQL instalado"

    # Iniciar PostgreSQL
    print_info "Iniciando PostgreSQL..."
    if command -v systemctl &> /dev/null; then
        sudo systemctl start postgresql 2>/dev/null || true
        sudo systemctl enable postgresql 2>/dev/null || true
    else
        sudo service postgresql start 2>/dev/null || true
    fi
    sleep 2
    print_success "PostgreSQL rodando"

    # Testar conexão
    print_info "Testando conexão..."
    if ! sudo -u "$POSTGRES_SUPERUSER" psql -c "SELECT 1;" > /dev/null 2>&1; then
        print_error "Não foi possível conectar ao PostgreSQL"
        exit 1
    fi
    print_success "Conexão OK"

    # Criar usuário
    print_info "Configurando usuário do banco..."
    USER_EXISTS=$(sudo -u "$POSTGRES_SUPERUSER" psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='$DB_USER';" 2>/dev/null || echo "0")
    if [[ "$USER_EXISTS" == "1" ]]; then
        sudo -u "$POSTGRES_SUPERUSER" psql -c "ALTER ROLE $DB_USER WITH PASSWORD '$DB_PASSWORD';" > /dev/null
        print_success "Usuário '$DB_USER' atualizado"
    else
        sudo -u "$POSTGRES_SUPERUSER" psql -c "CREATE ROLE $DB_USER WITH LOGIN PASSWORD '$DB_PASSWORD';" > /dev/null
        print_success "Usuário '$DB_USER' criado"
    fi

    # Criar bancos
    print_info "Criando bancos de dados..."
    for DB in "$DB_SERVICE" "$DB_EMULATOR"; do
        DB_EXISTS=$(sudo -u "$POSTGRES_SUPERUSER" psql -tAc "SELECT 1 FROM pg_database WHERE datname='$DB';" 2>/dev/null || echo "0")
        if [[ "$DB_EXISTS" == "1" ]]; then
            print_info "Banco '$DB' já existe"
        else
            sudo -u "$POSTGRES_SUPERUSER" psql -c "CREATE DATABASE $DB OWNER $DB_USER;" > /dev/null
            print_success "Banco '$DB' criado"
        fi
    done

    # Criar schemas
    print_info "Criando schemas..."
    sudo -u "$POSTGRES_SUPERUSER" psql -d "$DB_SERVICE" << EOF > /dev/null
CREATE SCHEMA IF NOT EXISTS service AUTHORIZATION $DB_USER;
GRANT ALL PRIVILEGES ON SCHEMA service TO $DB_USER;
ALTER DEFAULT PRIVILEGES IN SCHEMA service GRANT ALL ON TABLES TO $DB_USER;
ALTER DEFAULT PRIVILEGES IN SCHEMA service GRANT ALL ON SEQUENCES TO $DB_USER;
EOF

    sudo -u "$POSTGRES_SUPERUSER" psql -d "$DB_EMULATOR" << EOF > /dev/null
CREATE SCHEMA IF NOT EXISTS emulator AUTHORIZATION $DB_USER;
GRANT ALL PRIVILEGES ON SCHEMA emulator TO $DB_USER;
ALTER DEFAULT PRIVILEGES IN SCHEMA emulator GRANT ALL ON TABLES TO $DB_USER;
ALTER DEFAULT PRIVILEGES IN SCHEMA emulator GRANT ALL ON SEQUENCES TO $DB_USER;
EOF
    print_success "Schemas configurados"

    # Criar .env
    if [[ ! -f .env ]]; then
        print_info "Criando arquivo .env..."
        cat > .env << EOF
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

PG_HOST=$DB_HOST
PG_PORT=$DB_PORT
PG_USER=$DB_USER
PG_PASSWORD=$DB_PASSWORD
PG_DATABASE=$DB_SERVICE

DB_HOST=$DB_HOST
DB_PORT=$DB_PORT
DB_USER=$DB_USER
DB_PASSWORD=$DB_PASSWORD
DB_DATABASE=$DB_EMULATOR

WXS_HOST=172.20.112.1
WXS_PORT=1433
WXS_DATABASE=W_Access
WXS_USER=W-Access
WXS_PASSWORD=sua_senha_wxs
EOF
        print_success "Arquivo .env criado"
    else
        print_info "Arquivo .env já existe"
    fi

    # Compilar aplicação
    if command -v go &> /dev/null; then
        print_info "Compilando aplicação..."
        go mod download > /dev/null 2>&1 || true
        mkdir -p bin
        if go build -o bin/emulator-service cmd/emulator-service/main.go 2>/dev/null; then
            print_success "Aplicação compilada: bin/emulator-service"
        else
            print_warning "Erro na compilação (verifique dependências Go)"
        fi
    else
        print_warning "Go não encontrado. Pule a compilação."
    fi

else
    print_error "Modo inválido: $DEPLOY_MODE"
    print_info "Use: docker, native ou auto"
    exit 1
fi

# ============================================================
# RESUMO
# ============================================================

echo ""
echo -e "${GREEN}============================================================${NC}"
echo -e "${GREEN}Deploy Concluído!${NC}"
echo -e "${GREEN}============================================================${NC}"
echo ""
print_success "Modo: $DEPLOY_MODE"

if [[ "$DEPLOY_MODE" == "docker" ]]; then
    print_success "Containers Docker criados e iniciados"
    print_success "PostgreSQL: facial-emulator-db"
    print_success "Aplicação: facial-emulator-app"
    echo ""
    echo -e "${BLUE}Comandos úteis:${NC}"
    echo -e "  • Ver logs da app: ${GREEN}docker-compose logs -f app${NC}"
    echo -e "  • Ver logs do DB: ${GREEN}docker-compose logs -f postgres${NC}"
    echo -e "  • Parar tudo: ${GREEN}docker-compose down${NC}"
    echo -e "  • Reiniciar: ${GREEN}docker-compose restart${NC}"
    echo -e "  • Status: ${GREEN}docker-compose ps${NC}"
else
    print_success "PostgreSQL configurado"
    print_success "Bancos criados: $DB_SERVICE, $DB_EMULATOR"
    print_success "Schemas: service, emulator"
    print_success "Usuário: $DB_USER"
    print_success "Arquivo .env configurado"
    echo ""
    echo -e "${BLUE}Para iniciar a aplicação:${NC}"
    echo -e "  ${GREEN}./bin/emulator-service${NC}"
    echo -e "  ou"
    echo -e "  ${GREEN}go run cmd/emulator-service/main.go${NC}"
fi

echo ""
echo -e "${BLUE}Acesse a aplicação:${NC} ${GREEN}http://localhost:8080${NC}"
echo ""
