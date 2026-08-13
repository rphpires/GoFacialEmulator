#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log

echo "=============================================================="
echo "  GoFacialEmulator - Iniciando"
echo "=============================================================="

if ! docker info >/dev/null 2>&1; then
    echo "❌ O Docker nao esta rodando. Inicie o Docker e tente de novo."
    exit 1
fi

if ! docker image inspect gofacialemulator:1.0 >/dev/null 2>&1; then
    echo "❌ A aplicacao ainda nao foi instalada. Rode ./instalar.sh primeiro."
    exit 1
fi

echo "[1/2] Subindo os servicos ..."
if ! docker compose -f sistema/docker-compose.yml up -d >>"$LOG" 2>&1; then
    echo "❌ Falha ao subir os servicos — veja sistema/logs/instalacao.log"
    exit 1
fi

echo "[2/2] Aguardando a aplicacao responder ..."
for _ in $(seq 1 60); do
    if curl -s http://localhost:7070/monitoring/health/quick >/dev/null 2>&1; then
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
echo "   Veja o log com: docker compose -f sistema/docker-compose.yml logs app"
exit 1
