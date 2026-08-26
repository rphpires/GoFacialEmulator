package database

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v4"
)

// Instalação nova, sem W-Access configurado de jeito nenhum (nem
// config.yaml, nem WXS_*): sem linha em wxs_settings e sem configuração
// efetiva, o sync fica desligado.
func TestGetSyncEnabledSemLinhaESemWxsDevolveFalso(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{err: pgx.ErrNoRows}}

	ligado, err := GetSyncEnabled(context.Background(), db, false)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if ligado {
		t.Error("sem W-Access configurado o sync tem que estar desligado")
	}
}

// Instalação que configurou o W-Access via configs/config.yaml ou
// WXS_*/WXS_DB_* (cmd/emulator-service/main.go monta wxsDB nesse caso) mas
// nunca passou pela tela de configurações: sem linha em wxs_settings, mas
// com W-Access de fato configurado, o sync tem que continuar ligado —
// era exatamente essa instalação que uma migração ingênua desligaria calada.
func TestGetSyncEnabledSemLinhaComWxsConfiguradoDevolveTrue(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{err: pgx.ErrNoRows}}

	ligado, err := GetSyncEnabled(context.Background(), db, true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !ligado {
		t.Error("com W-Access configurado e sem linha explícita, o sync tem que seguir configurado")
	}
}

// Um valor explicitamente gravado (aqui, false) manda mais que
// wxsConfigurado nos dois sentidos: uma vez que o operador mexeu no toggle
// pela tela, a leitura tem que respeitar essa escolha.
func TestGetSyncEnabledValorGravadoBateWxsConfigurado(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{valores: []interface{}{false}}}

	ligado, err := GetSyncEnabled(context.Background(), db, true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if ligado {
		t.Error("valor gravado false tem que vencer, mesmo com wxsConfigurado=true")
	}
}

func TestGetSyncEnabledLeOValorGravado(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{valores: []interface{}{true}}}

	ligado, err := GetSyncEnabled(context.Background(), db, false)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !ligado {
		t.Error("quero ligado")
	}
}

// Sem nenhuma linha gravada, o toggle não pode ficar preso: SetSyncEnabled
// tem que criar a linha (host vazio) em vez de recusar, senão o operador que
// desligou o sync antes de configurar o W-Access não teria como religar.
func TestSetSyncEnabledSemLinhaInsereNovaLinha(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{err: pgx.ErrNoRows}}

	if err := SetSyncEnabled(context.Background(), db, true); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(db.execs) != 1 {
		t.Fatalf("quero 1 Exec, tenho %d: %v", len(db.execs), db.execs)
	}
	if !strings.Contains(db.execs[0], "INSERT INTO service.wxs_settings") {
		t.Errorf("quero um INSERT em service.wxs_settings, tenho %q", db.execs[0])
	}
}
