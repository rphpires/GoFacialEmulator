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
if ! docker compose -f "$COMPOSE" up -d postgres >>"$LOG" 2>&1; then
    echo
    echo "❌ Falha ao preparar o banco — veja sistema/logs/instalacao.log"
    exit 1
fi

# "docker compose up -d" só confirma que o container começou a subir, não
# que o banco já aceita conexões. --wait resolveria isso, mas exige Docker
# Compose 2.17+; o piso do pacote é 2.0, então esperamos manualmente pelo
# healthcheck que já existe em docker-compose.yml.
banco_pronto=0
for _ in $(seq 1 120); do
    status=$(docker inspect -f '{{.State.Health.Status}}' facial-emulator-db 2>/dev/null || true)
    if [ "$status" = "healthy" ]; then
        banco_pronto=1
        break
    fi
    sleep 1
done
if [ "$banco_pronto" -ne 1 ]; then
    echo
    echo "❌ O banco nao ficou pronto a tempo — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Banco preparado."

echo
echo "✅ Instalado. Rode ./iniciar.sh"
