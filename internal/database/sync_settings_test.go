package database

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v4"
)

// Instalação nova não tem linha em wxs_settings. Isso não é erro: é o
// estado de quem nunca configurou o W-Access, e o sync está desligado.
func TestGetSyncEnabledSemLinhaDevolveFalso(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{err: pgx.ErrNoRows}}

	ligado, err := GetSyncEnabled(context.Background(), db)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if ligado {
		t.Error("sem configuração de W-Access o sync tem que estar desligado")
	}
}

func TestGetSyncEnabledLeOValorGravado(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{valores: []interface{}{true}}}

	ligado, err := GetSyncEnabled(context.Background(), db)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !ligado {
		t.Error("quero ligado")
	}
}

func TestSetSyncEnabledSemLinhaRecusa(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{err: pgx.ErrNoRows}}

	err := SetSyncEnabled(context.Background(), db, true)
	if err != ErrNoWxsSettings {
		t.Errorf("quero ErrNoWxsSettings, tenho %v", err)
	}
}
