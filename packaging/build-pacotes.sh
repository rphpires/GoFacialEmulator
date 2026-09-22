#!/usr/bin/env bash
# Equivalente Linux de build-pacotes.bat: gera os mesmos ZIPs em packaging/.out.
# Uso: packaging/build-pacotes.sh [docker|windows|linux|manuais|todos]
set -euo pipefail
cd "$(dirname "$0")/.."

ALVO="${1:-todos}"
OUT="packaging/.out"
mkdir -p "$OUT"

erro() { echo "[ERRO] $*" >&2; exit 1; }

command -v go  >/dev/null || erro "Go nao encontrado. Instale em https://go.dev/dl/"
command -v zip >/dev/null || erro "zip nao encontrado. Instale com: sudo apt install zip"

# zipar STAGE ZIP: compacta o conteudo de STAGE (nao a pasta) em ZIP.
zipar() {
    local stage="$1" destino
    destino="$(pwd)/$2"
    rm -f "$destino"
    (cd "$stage" && zip -qr "$destino" .)
}

build_manuais() {
    echo
    echo "[manuais] Montando e imprimindo os tres PDFs ..."
    command -v npm >/dev/null || erro "npm nao encontrado. Instale o Node.js em https://nodejs.org/"
    if [ ! -d docs/manual/node_modules ]; then
        echo "[manuais] Instalando dependencias (uma vez) ..."
        (cd docs/manual && npm install --ignore-scripts)
    fi
    (cd docs/manual && npm run manuais) || erro "Falha ao gerar os manuais."
    echo "[manuais] OK: docs/manual/.out/MANUAL-*.pdf"
}

build_docker() {
    echo
    echo "[docker] Construindo a imagem ..."
    docker build -t gofacialemulator:1.0 . || erro "docker build falhou."

    local stage="$OUT/docker"
    rm -rf "$stage"
    mkdir -p "$stage/sistema/logs"

    echo "[docker] Exportando a imagem ..."
    docker save -o "$stage/sistema/gofacialemulator-imagem.tar" gofacialemulator:1.0 \
        || erro "docker save falhou."

    cp packaging/docker/docker-compose.yml packaging/docker/docker-compose.linux.yml "$stage/sistema/"
    cp packaging/docker/{INSTALAR.bat,INICIAR.bat,PARAR.bat,instalar.sh,iniciar.sh,parar.sh,LEIA-ME.txt} "$stage/"
    chmod +x "$stage"/*.sh
    if [ -f docs/manual/.out/MANUAL-docker.pdf ]; then
        cp docs/manual/.out/MANUAL-docker.pdf "$stage/MANUAL.pdf"
    fi

    zipar "$stage" "$OUT/GoFacialEmulator-docker.zip" || erro "Falha ao gerar o ZIP."
    echo "[docker] OK: $OUT/GoFacialEmulator-docker.zip"
}

build_windows() {
    local pgcache=".build-cache/postgres-portable"
    if [ ! -f "$pgcache/bin/postgres.exe" ]; then
        echo "[windows] Baixando o PostgreSQL portatil (uma vez) ..."
        command -v curl  >/dev/null || erro "curl nao encontrado."
        command -v unzip >/dev/null || erro "unzip nao encontrado."
        mkdir -p .build-cache
        rm -rf .build-cache/pg "$pgcache"
        curl -fL -o .build-cache/pg.zip \
            https://get.enterprisedb.com/postgresql/postgresql-15.8-1-windows-x64-binaries.zip
        unzip -q .build-cache/pg.zip -d .build-cache/pg
        mv .build-cache/pg/pgsql "$pgcache"
        rm -rf .build-cache/pg.zip .build-cache/pg
        [ -f "$pgcache/bin/postgres.exe" ] || erro "Falha ao baixar o PostgreSQL portatil."
    fi

    local stage="$OUT/windows"
    rm -rf "$stage"
    mkdir -p "$stage/sistema/logs" "$stage/sistema/configs" "$stage/sistema/postgres"

    echo "[windows] Compilando ..."
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
        go build -ldflags="-s -w" -o "$stage/sistema/emulator-service.exe" ./cmd/emulator-service \
        || erro "Falha na compilacao."

    echo "[windows] Copiando o PostgreSQL portatil ..."
    cp -r "$pgcache/bin" "$pgcache/lib" "$pgcache/share" "$stage/sistema/postgres/"

    cp packaging/windows/config.yaml "$stage/sistema/configs/config.yaml"
    cp packaging/windows/{INSTALAR.bat,INICIAR.bat,PARAR.bat,LEIA-ME.txt} "$stage/"
    if [ -f docs/manual/.out/MANUAL-windows.pdf ]; then
        cp docs/manual/.out/MANUAL-windows.pdf "$stage/MANUAL.pdf"
    fi

    zipar "$stage" "$OUT/GoFacialEmulator-windows.zip" || erro "Falha ao gerar o ZIP."
    echo "[windows] OK: $OUT/GoFacialEmulator-windows.zip"
}

build_linux() {
    local stage="$OUT/linux"
    rm -rf "$stage"
    mkdir -p "$stage/sistema/logs" "$stage/sistema/configs"

    echo "[linux] Compilando ..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -ldflags="-s -w" -o "$stage/sistema/emulator-service" ./cmd/emulator-service \
        || erro "Falha na compilacao."

    cp packaging/linux/config.yaml "$stage/sistema/configs/config.yaml"
    cp packaging/linux/{instalar.sh,iniciar.sh,parar.sh,LEIA-ME.txt} "$stage/"
    chmod +x "$stage"/*.sh
    if [ -f docs/manual/.out/MANUAL-linux.pdf ]; then
        cp docs/manual/.out/MANUAL-linux.pdf "$stage/MANUAL.pdf"
    fi

    zipar "$stage" "$OUT/GoFacialEmulator-linux.zip" || erro "Falha ao gerar o ZIP."
    echo "[linux] OK: $OUT/GoFacialEmulator-linux.zip"
}

case "$ALVO" in
    docker)  build_docker ;;
    windows) build_windows ;;
    linux)   build_linux ;;
    manuais) build_manuais ;;
    todos)   build_manuais; build_docker; build_windows; build_linux ;;
    *) echo "[ERRO] Alvo invalido: $ALVO" >&2
       echo "Uso: build-pacotes.sh [docker|windows|linux|manuais|todos]" >&2
       exit 1 ;;
esac

echo
echo "Pacotes gerados em $OUT/"
ls -1 "$OUT"/*.zip 2>/dev/null || true
