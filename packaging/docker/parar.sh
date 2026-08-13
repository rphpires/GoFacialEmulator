#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log

echo "Parando o GoFacialEmulator ..."
if ! docker compose -f sistema/docker-compose.yml stop >>"$LOG" 2>&1; then
    echo
    echo "❌ Nao foi possivel parar — veja sistema/logs/instalacao.log"
    exit 1
fi
echo
echo "✅ Parado. Os dados continuam salvos."
