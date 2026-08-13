#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

echo "Parando o GoFacialEmulator ..."
docker compose -f sistema/docker-compose.yml stop >/dev/null 2>&1
echo
echo "✅ Parado. Os dados continuam salvos."
