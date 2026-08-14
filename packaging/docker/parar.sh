#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

# Em Linux usamos a rede do host, que não tem limite de portas. No Docker
# Desktop (Windows/Mac) a rede de host é experimental, então lá vale o
# compose que publica por faixa.
COMPOSE=sistema/docker-compose.yml
if [ "$(uname -s)" = "Linux" ] && ! grep -qiE "microsoft|wsl" /proc/version 2>/dev/null; then
    COMPOSE=sistema/docker-compose.linux.yml
fi

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log

echo "Parando o GoFacialEmulator ..."
if ! docker compose -f "$COMPOSE" stop >>"$LOG" 2>&1; then
    echo
    echo "❌ Nao foi possivel parar — veja sistema/logs/instalacao.log"
    exit 1
fi
echo
echo "✅ Parado. Os dados continuam salvos."
