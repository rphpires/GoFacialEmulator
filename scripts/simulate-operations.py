#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
W-Access (Invenzi) — Simulador de operação normal sobre usuários do emulador.

O que faz (loop infinito até Ctrl+C):
  1. Consulta usuários marcados com CHAux.AuxChk03 = 1 (recém-adicionados).
  2. Para cada usuário:
       a. snapshot dos níveis de acesso atuais (CHAccessLevels)
       b. REMOVE todos os níveis
       c. aguarda 1 segundo
       d. RE-ADICIONA os mesmos níveis (mesmas validades)
       e. executa dbo.wxsp_DownloadCardholder -> envia o usuário ao gerenciador
  3. Pausa entre ciclos e repete.

Objetivo: gerar carga/movimento realista (remoção+readição de nível + download)
para validar o emulador facial e o caminho de sincronização com o gerenciador.

Requisitos:
  - Python 3.8+
  - pip install pyodbc
  - ODBC Driver 17 ou 18 for SQL Server instalado.

Autenticação: Windows (Trusted_Connection). Roda com o usuário Windows atual.

AVISO: este script ALTERA dados (remove/readiciona níveis) e dispara download
real ao gerenciador. Use apenas no ambiente de emulação/teste.
"""

import sys
import time
import signal
from datetime import datetime

try:
    import pyodbc
except ImportError:
    sys.exit("pyodbc não instalado. Rode: pip install pyodbc")

# ====================================================================
# CONFIG — edite apenas este bloco
# ====================================================================
SERVER   = "RPH-SRV"          # host do SQL Server
DATABASE = "W_ACCESS"
# Drivers tentados em ordem (o primeiro instalado vence)
ODBC_DRIVERS = ["ODBC Driver 18 for SQL Server", "ODBC Driver 17 for SQL Server"]

# Parâmetros do wxsp_DownloadCardholder"
DOWNLOAD_SERVER_NAME = "localhost"   # @serverName
DOWNLOAD_COMMKEY     = 0             # @CommKey (ajuste se o gerenciador exigir chave real)

# Filtro dos usuários a simular
CHTYPE_FILTER = None    # ex.: 6 para só CHType=6; None = qualquer CHType com AuxChk03=1

# Tempos (segundos)
REMOVE_READD_WAIT = 1.0   # espera entre remover e readicionar (pedido: 1s)
CYCLE_PAUSE       = 2.0   # pausa entre ciclos completos
EMPTY_PAUSE       = 5.0   # pausa quando não há usuários marcados

# ====================================================================
_stop = False


def _handle_sigint(signum, frame):
    global _stop
    _stop = True
    print("\n[!] Ctrl+C recebido — encerrando após a operação atual...")


def log(msg):
    print(f"[{datetime.now():%H:%M:%S}] {msg}", flush=True)


def connect():
    """Abre conexão Trusted. Tenta os drivers ODBC em ordem."""
    last_err = None
    for drv in ODBC_DRIVERS:
        try:
            cn = pyodbc.connect(
                f"DRIVER={{{drv}}};SERVER={SERVER};DATABASE={DATABASE};"
                f"Trusted_Connection=yes;TrustServerCertificate=yes;",
                autocommit=True,
            )
            log(f"Conectado via '{drv}' em {SERVER}/{DATABASE}")
            return cn
        except pyodbc.Error as e:
            last_err = e
    raise SystemExit(f"Falha ao conectar (drivers tentados: {ODBC_DRIVERS}). Erro: {last_err}")


def fetch_flagged_users(cur):
    """CHIDs com AuxChk03 = 1 (recém-adicionados), opcionalmente por CHType."""
    sql = (
        "SELECT m.CHID, m.FirstName "
        "FROM dbo.CHAux a "
        "JOIN dbo.CHMain m ON m.CHID = a.CHID "
        "WHERE a.AuxChk03 = 1 "
    )
    params = []
    if CHTYPE_FILTER is not None:
        sql += "AND m.CHType = ? "
        params.append(CHTYPE_FILTER)
    sql += "ORDER BY m.CHID"
    cur.execute(sql, *params)
    return cur.fetchall()


def fetch_levels(cur, chid):
    """Snapshot dos níveis de acesso do CH (com validades)."""
    cur.execute(
        "SELECT AccessLevelID, AccessLevelStartValidity, AccessLevelEndValidity "
        "FROM dbo.CHAccessLevels WHERE CHID = ?",
        chid,
    )
    return cur.fetchall()


def remove_levels(cur, chid):
    cur.execute("DELETE FROM dbo.CHAccessLevels WHERE CHID = ?", chid)
    return cur.rowcount


def readd_levels(cur, chid, levels):
    for lvl in levels:
        cur.execute(
            "IF NOT EXISTS (SELECT 1 FROM dbo.CHAccessLevels "
            "               WHERE CHID = ? AND AccessLevelID = ?) "
            "INSERT INTO dbo.CHAccessLevels "
            "  (CHID, AccessLevelID, AccessLevelStartValidity, AccessLevelEndValidity) "
            "VALUES (?, ?, ?, ?)",
            chid, lvl.AccessLevelID,
            chid, lvl.AccessLevelID, lvl.AccessLevelStartValidity, lvl.AccessLevelEndValidity,
        )
    return len(levels)


def download_cardholder(cur, chid):
    """Executa wxsp_DownloadCardholder e devolve @reply."""
    cur.execute(
        "SET NOCOUNT ON; "
        "DECLARE @reply nvarchar(4000); "
        "EXEC dbo.wxsp_DownloadCardholder "
        "     @serverName = ?, @CHID = ?, @CommKey = ?, @reply = @reply OUTPUT; "
        "SELECT @reply AS reply;",
        DOWNLOAD_SERVER_NAME, chid, DOWNLOAD_COMMKEY,
    )
    row = cur.fetchone()
    return row.reply if row else None


def run_cycle(cur, cycle_no):
    users = fetch_flagged_users(cur)
    if not users:
        log(f"Ciclo {cycle_no}: nenhum usuário com AuxChk03=1. Aguardando {EMPTY_PAUSE}s.")
        return False

    log(f"Ciclo {cycle_no}: {len(users)} usuário(s).")
    for u in users:
        if _stop:
            break
        # chid = u.CHID
        chid = 1
        try:
            levels = fetch_levels(cur, chid)
            removed = remove_levels(cur, chid)
            reply = download_cardholder(cur, chid)
            time.sleep(REMOVE_READD_WAIT)
            added = readd_levels(cur, chid, levels)
            reply = download_cardholder(cur, chid)
            log(f"  CHID={chid} '{u.FirstName.strip()}' | "
                f"níveis: -{removed}/+{added} | download.reply={reply!r}")
        except pyodbc.Error as e:
            log(f"  CHID={chid} ERRO -> {e}")
    return True


def main():
    signal.signal(signal.SIGINT, _handle_sigint)
    cn = connect()
    cur = cn.cursor()
    log("Simulador iniciado. Ctrl+C para parar.")

    cycle = 0
    try:
        while not _stop:
            cycle += 1
            had_users = run_cycle(cur, cycle)
            if _stop:
                break
            time.sleep(CYCLE_PAUSE if had_users else EMPTY_PAUSE)
    finally:
        cur.close()
        cn.close()
        log("Conexão fechada. Encerrado.")


if __name__ == "__main__":
    main()
