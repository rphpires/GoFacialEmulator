#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

mkdir -p sistema/logs
LOG=sistema/logs/instalacao.log
: > "$LOG"

echo "=============================================================="
echo "  GoFacialEmulator - Instalacao (Linux)"
echo "=============================================================="

if [ "$(id -u)" -ne 0 ]; then
    echo
    echo "❌ Rode com sudo:  sudo ./instalar.sh"
    exit 1
fi

if grep -qiE "microsoft|wsl" /proc/version 2>/dev/null; then
    NO_WSL=1
    echo "      Ambiente detectado: WSL"
else
    NO_WSL=0
    echo "      Ambiente detectado: Linux"
fi

echo
echo "Qual faixa de portas os emuladores vao usar?"
echo "As portas vem do W-Access (campo BaseCommPort de cada controlador)."
printf "Faixa [4000-4499]: "
read -r FAIXA
FAIXA="${FAIXA:-4000-4499}"

case "$FAIXA" in
    *-*) PORTA_INICIO="${FAIXA%%-*}"; PORTA_FIM="${FAIXA##*-}" ;;
    *)   PORTA_INICIO="$FAIXA"; PORTA_FIM="$FAIXA" ;;
esac

if ! echo "$PORTA_INICIO" | grep -qE '^[0-9]+$' \
   || ! echo "$PORTA_FIM" | grep -qE '^[0-9]+$' \
   || [ "$PORTA_INICIO" -gt "$PORTA_FIM" ]; then
    echo
    echo "❌ Faixa invalida: $FAIXA. Use o formato 4000-4499."
    exit 1
fi

QTD_PORTAS=$((PORTA_FIM - PORTA_INICIO + 1))

echo "[1/5] Instalando o PostgreSQL ..."
if ! command -v psql >/dev/null 2>&1; then
    apt-get update >>"$LOG" 2>&1
    if ! apt-get install -y postgresql postgresql-contrib >>"$LOG" 2>&1; then
        echo
        echo "❌ Falha ao instalar o PostgreSQL — veja sistema/logs/instalacao.log"
        exit 1
    fi
fi
echo "      PostgreSQL OK."

echo "[2/5] Ligando o banco ..."
if [ "$NO_WSL" -eq 1 ]; then
    service postgresql start >>"$LOG" 2>&1
else
    systemctl enable --now postgresql >>"$LOG" 2>&1
fi

for _ in $(seq 1 30); do
    if su - postgres -c "pg_isready" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

if ! su - postgres -c "pg_isready" >/dev/null 2>&1; then
    echo
    echo "❌ O banco nao ficou pronto — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Banco ligado."

echo "[3/5] Criando o usuario da aplicacao ..."
su - postgres -c "psql -tAc \"SELECT 1 FROM pg_roles WHERE rolname='emulator'\"" 2>>"$LOG" | grep -q 1 \
  || su - postgres -c "psql -c \"CREATE USER emulator WITH PASSWORD 'emulator123';\"" >>"$LOG" 2>&1

su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='emulator_db'\"" 2>>"$LOG" | grep -q 1 \
  || su - postgres -c "psql -c \"CREATE DATABASE emulator_db OWNER emulator;\"" >>"$LOG" 2>&1

if ! su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='emulator_db'\"" 2>>"$LOG" | grep -q 1; then
    echo
    echo "❌ Falha ao criar o banco emulator_db — veja sistema/logs/instalacao.log"
    exit 1
fi
echo "      Usuario e banco OK."

if ! chmod +x sistema/emulator-service iniciar.sh parar.sh; then
    echo
    echo "❌ Falha ao preparar as permissoes dos scripts — veja sistema/logs/instalacao.log"
    exit 1
fi

echo "[4/5] Liberando as portas no firewall ..."
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi "^Status: active"; then
    if ufw allow "${PORTA_INICIO}:${PORTA_FIM}/tcp" >>"$LOG" 2>&1; then
        echo "      ufw: liberado ${PORTA_INICIO}-${PORTA_FIM}/tcp."
    else
        echo "      ⚠  Nao foi possivel liberar no ufw — veja sistema/logs/instalacao.log"
    fi
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    if firewall-cmd --permanent --add-port="${PORTA_INICIO}-${PORTA_FIM}/tcp" >>"$LOG" 2>&1 \
       && firewall-cmd --reload >>"$LOG" 2>&1; then
        echo "      firewalld: liberado ${PORTA_INICIO}-${PORTA_FIM}/tcp."
    else
        echo "      ⚠  Nao foi possivel liberar no firewalld — veja sistema/logs/instalacao.log"
    fi
else
    echo "      Nenhum firewall ativo. Nada a fazer."
fi

echo "[5/5] Conferindo o limite de arquivos abertos ..."
# Cada emulador abre um socket de escuta e mantém conexões. A folga de
# 2 vezes o número de portas mais 1024 cobre as conexões simultâneas.
NECESSARIO=$((QTD_PORTAS * 2 + 1024))

# O script roda sob sudo, mas quem inicia a aplicacao depois e o usuario
# comum, via ./iniciar.sh. O limite que importa e o dele, nao o do root.
USUARIO_ALVO="${SUDO_USER:-}"
if [ -z "$USUARIO_ALVO" ]; then
    USUARIO_ALVO="$(logname 2>/dev/null || true)"
fi

ATUAL=""
if [ -n "$USUARIO_ALVO" ]; then
    ATUAL="$(su - "$USUARIO_ALVO" -c 'ulimit -n' 2>/dev/null || true)"
fi
if ! echo "${ATUAL:-}" | grep -qE '^[0-9]+$'; then
    ATUAL="$(ulimit -n)"
    USUARIO_ALVO="root"
fi

if [ "$ATUAL" -lt "$NECESSARIO" ]; then
    cat > /etc/security/limits.d/gofacialemulator.conf <<EOF
# Gerado pelo instalar.sh do GoFacialEmulator para $QTD_PORTAS emuladores.
* soft nofile $NECESSARIO
* hard nofile $NECESSARIO
EOF
    echo "      Limite atual do usuario $USUARIO_ALVO ($ATUAL) e menor que o necessario ($NECESSARIO)."
    echo "      ⚠  Ajustado em /etc/security/limits.d/gofacialemulator.conf."
    echo "         Saia e entre de novo na sessao antes de rodar ./iniciar.sh."
else
    echo "      Limite atual do usuario $USUARIO_ALVO ($ATUAL) e suficiente."
fi

if [ "$NO_WSL" -eq 1 ] && [ ! -e /sys/class/net/loopback0 ]; then
    echo
    echo "⚠  ATENCAO — esta WSL nao esta em modo espelhado."
    echo "   Sem isso, so ESTA maquina alcanca os emuladores: o Site"
    echo "   Controller em outro computador nao vai conseguir conectar."
    echo
    echo "   Para corrigir, crie ou edite o arquivo .wslconfig na pasta"
    echo "   do seu usuario no Windows (C:\\Users\\SEU_USUARIO\\.wslconfig)"
    echo "   com o conteudo:"
    echo
    echo "     [wsl2]"
    echo "     networkingMode=mirrored"
    echo
    echo "   Depois, no PowerShell do Windows: wsl --shutdown"
    echo "   e abra a WSL de novo."
    echo
fi

echo
echo "✅ Instalado. Rode ./iniciar.sh"
