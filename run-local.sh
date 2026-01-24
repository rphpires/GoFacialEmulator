#!/bin/bash

echo "========================================"
echo " GoFacialEmulator - Desenvolvimento Local"
echo "========================================"
echo ""

# Verificar se o banco está rodando
if ! docker ps | grep -q facial-emulator-db; then
    echo "[AVISO] Banco de dados não está rodando!"
    echo "Iniciando PostgreSQL..."
    docker-compose -f docker-compose.db-only.yml up -d
    echo "Aguardando banco inicializar..."
    sleep 5
fi

echo "[OK] Banco de dados rodando"
echo ""
echo "Iniciando aplicação localmente..."
echo "Usando config: configs/config.local.yaml"
echo ""

# Executar aplicação com config local
go run cmd/emulator-service/main.go -config=configs/config.local.yaml
