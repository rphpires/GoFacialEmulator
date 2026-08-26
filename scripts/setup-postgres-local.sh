#!/bin/bash
# Setup Postgres nativo para GoFacialEmulator (Ubuntu 24.04).
# Idempotente: pode rodar de novo sem quebrar.
# Rodar com: sudo bash scripts/setup-postgres-local.sh
set -euo pipefail

DB_USER="emulator"
DB_PASS="emulator123"
DB_NAME="emulator_db"

echo "==> 1. Instalando PostgreSQL..."
if ! command -v psql >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y postgresql postgresql-contrib
else
    echo "    PostgreSQL ja instalado, pulando apt."
fi

# Detectar versao/cluster instalado
PG_VER=$(ls /etc/postgresql/ | sort -n | tail -1)
echo "==> Cluster detectado: versao $PG_VER"

echo "==> 2. Garantindo cluster ativo..."
pg_ctlcluster "$PG_VER" main start 2>/dev/null || true
systemctl enable postgresql 2>/dev/null || true
# Esperar aceitar conexoes
for i in $(seq 1 15); do
    if pg_isready -q; then break; fi
    sleep 1
done

echo "==> 3. Criando role '$DB_USER' e banco '$DB_NAME'..."
sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_USER}') THEN
        CREATE ROLE ${DB_USER} LOGIN PASSWORD '${DB_PASS}' CREATEDB;
    ELSE
        ALTER ROLE ${DB_USER} LOGIN PASSWORD '${DB_PASS}' CREATEDB;
    END IF;
END
\$\$;
SQL

# Criar banco se nao existir (CREATE DATABASE nao roda em DO block)
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
    sudo -u postgres createdb -O "${DB_USER}" "${DB_NAME}"
    echo "    Banco ${DB_NAME} criado."
else
    echo "    Banco ${DB_NAME} ja existe."
fi
sudo -u postgres psql -v ON_ERROR_STOP=1 -c "ALTER DATABASE ${DB_NAME} OWNER TO ${DB_USER};"

echo "==> 4. Aplicando tuning (ALTER SYSTEM)..."
# Calibrado: 15GB RAM, 10 cores, workload = muitos emuladores, muitas queries pequenas.
# max_connections alto pois app cria 2 pool managers (ServiceDB+EmulatorDB) que dobram conexoes.
sudo -u postgres psql -v ON_ERROR_STOP=1 <<'SQL'
ALTER SYSTEM SET max_connections = 400;
ALTER SYSTEM SET shared_buffers = '4GB';
ALTER SYSTEM SET effective_cache_size = '10GB';
ALTER SYSTEM SET work_mem = '8MB';
ALTER SYSTEM SET maintenance_work_mem = '512MB';
ALTER SYSTEM SET synchronous_commit = 'off';
ALTER SYSTEM SET wal_buffers = '16MB';
ALTER SYSTEM SET max_wal_size = '4GB';
ALTER SYSTEM SET min_wal_size = '1GB';
ALTER SYSTEM SET checkpoint_completion_target = '0.9';
ALTER SYSTEM SET random_page_cost = 1.1;
ALTER SYSTEM SET effective_io_concurrency = 200;
SQL

echo "==> 5. Reiniciando para aplicar max_connections/shared_buffers..."
pg_ctlcluster "$PG_VER" main restart
for i in $(seq 1 15); do
    if pg_isready -q; then break; fi
    sleep 1
done

echo "==> 6. Testando conexao como '$DB_USER' via TCP..."
if PGPASSWORD="${DB_PASS}" psql -h localhost -U "${DB_USER}" -d "${DB_NAME}" -c "SELECT 'conexao OK' AS status, current_setting('max_connections') AS max_conns;"; then
    echo ""
    echo "==> SETUP COMPLETO. Banco pronto."
    echo "    Rode a app com: ./start_server.sh   (ou bash run-local.sh)"
else
    echo "!! Conexao TCP falhou. Verifique pg_hba.conf (auth localhost)."
    exit 1
fi
