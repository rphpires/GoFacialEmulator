#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log
: > "$LOG"

echo "=============================================================="
echo "  GoFacialEmulator - Instalacao (Docker)"
echo "=============================================================="

echo "[1/3] Verificando o Docker ..."
if ! docker info >>"$LOG" 2>&1; then
    echo
    echo "❌ O Docker nao esta rodando ou seu usuario nao tem permissao."
    echo "   Inicie o Docker e rode ./instalar.sh de novo."
    exit 1
fi
echo "      Docker OK."

echo "[2/3] Carregando a aplicacao ..."
if ! docker load -i sistema/gofacialemulator-imagem.tar >>"$LOG" 2>&1; then
    echo
    echo "❌ Falha ao carregar a aplicacao — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Aplicacao carregada."

echo "[3/3] Preparando o banco de dados ..."
if ! docker compose -f sistema/docker-compose.yml up -d postgres >>"$LOG" 2>&1; then
    echo
    echo "❌ Falha ao preparar o banco — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Banco preparado."

echo
echo "✅ Instalado. Rode ./iniciar.sh"
