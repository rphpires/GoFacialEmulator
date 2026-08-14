#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
PIDFILE=sistema/logs/app.pid

echo "=============================================================="
echo "  GoFacialEmulator - Iniciando"
echo "=============================================================="

if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
    echo "❌ Ja esta rodando. Abra http://localhost:7070 ou rode ./parar.sh antes."
    exit 1
fi

echo "[1/3] Ligando o banco ..."
if grep -qiE "microsoft|wsl" /proc/version 2>/dev/null; then
    sudo service postgresql start >/dev/null 2>&1
else
    sudo systemctl start postgresql >/dev/null 2>&1
fi

echo "[2/3] Iniciando a aplicacao ..."
cd sistema
nohup ./emulator-service -config configs/config.yaml >logs/app.out 2>&1 &
echo $! > logs/app.pid
cd ..

echo "[3/3] Aguardando a aplicacao responder ..."
for _ in $(seq 1 60); do
    if curl -sf http://localhost:7070/monitoring/health/quick >/dev/null 2>&1; then
        echo
        echo "✅ Rodando em http://localhost:7070"
        echo
        echo "   Para parar: ./parar.sh"
        exit 0
    fi
    sleep 1
done

echo
echo "❌ A aplicacao nao respondeu em 60 segundos."
echo "   Veja sistema/logs/trace.log e sistema/logs/app.out"
exit 1
