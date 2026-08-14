#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log
: > "$LOG"

echo "=============================================================="
echo "  GoFacialEmulator - Instalacao (Linux)"
echo "=============================================================="

if [ "$(id -u)" -ne 0 ]; then
    echo
    echo "❌ Rode com sudo:  sudo ./instalar.sh"
    exit 1
fi

if grep -qiE "microsoft|wsl" /proc/version 2>/dev/null; then
    NO_WSL=1
    echo "      Ambiente detectado: WSL"
else
    NO_WSL=0
    echo "      Ambiente detectado: Linux"
fi

echo "[1/3] Instalando o PostgreSQL ..."
if ! command -v psql >/dev/null 2>&1; then
    apt-get update >>"$LOG" 2>&1
    if ! apt-get install -y postgresql postgresql-contrib >>"$LOG" 2>&1; then
        echo
        echo "❌ Falha ao instalar o PostgreSQL — veja sistema/logs/instalacao.log"
        exit 1
    fi
fi
echo "      PostgreSQL OK."

echo "[2/3] Ligando o banco ..."
if [ "$NO_WSL" -eq 1 ]; then
    service postgresql start >>"$LOG" 2>&1
else
    systemctl enable --now postgresql >>"$LOG" 2>&1
fi

for _ in $(seq 1 30); do
    if su - postgres -c "pg_isready" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

if ! su - postgres -c "pg_isready" >/dev/null 2>&1; then
    echo
    echo "❌ O banco nao ficou pronto — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Banco ligado."

echo "[3/3] Criando o usuario da aplicacao ..."
su - postgres -c "psql -tAc \"SELECT 1 FROM pg_roles WHERE rolname='emulator'\"" 2>>"$LOG" | grep -q 1 \
  || su - postgres -c "psql -c \"CREATE USER emulator WITH PASSWORD 'emulator123';\"" >>"$LOG" 2>&1

su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='emulator_db'\"" 2>>"$LOG" | grep -q 1 \
  || su - postgres -c "psql -c \"CREATE DATABASE emulator_db OWNER emulator;\"" >>"$LOG" 2>&1

if ! su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='emulator_db'\"" 2>>"$LOG" | grep -q 1; then
    echo
    echo "❌ Falha ao criar o banco emulator_db — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Usuario e banco OK."

if ! chmod +x sistema/emulator-service iniciar.sh parar.sh; then
    echo
    echo "❌ Falha ao preparar as permissoes dos scripts — veja sistema/logs/instalacao.log"
    exit 1
fi

echo
echo "✅ Instalado. Rode ./iniciar.sh"
