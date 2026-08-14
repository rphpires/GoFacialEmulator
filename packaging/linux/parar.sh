#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

PIDFILE=sistema/logs/app.pid

echo "Parando a aplicacao ..."
if [ -f "$PIDFILE" ]; then
    kill "$(cat "$PIDFILE")" 2>/dev/null
    rm -f "$PIDFILE"
else
    pkill -f "emulator-service -config" 2>/dev/null
fi

echo
echo "✅ Parado. Os dados continuam salvos."
echo "   O banco de dados continua ligado (é um serviço do sistema)."
