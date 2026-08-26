package database

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

// pendingMigrations é a parte do runner que dá para testar sem banco:
// dado o conteúdo embutido e o conjunto já aplicado, quais faltam e em
// que ordem.
func TestPendingMigrationsOrdenaEIgnoraAplicadas(t *testing.T) {
	fsys := fstest.MapFS{
		"V003_terceira.sql": {Data: []byte("SELECT 3;")},
		"V001_baseline.sql": {Data: []byte("SELECT 1;")},
		"V002_segunda.sql":  {Data: []byte("SELECT 2;")},
		"leiame.txt":        {Data: []byte("não é migração")},
	}

	pend, err := pendingMigrations(fsys, map[string]bool{"V001": true})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(pend) != 2 {
		t.Fatalf("quero 2 pendentes, tenho %d: %+v", len(pend), pend)
	}
	if pend[0].Version != "V002" || pend[1].Version != "V003" {
		t.Errorf("ordem errada: %s, %s", pend[0].Version, pend[1].Version)
	}
	if pend[0].SQL != "SELECT 2;" {
		t.Errorf("SQL de V002: %q", pend[0].SQL)
	}
}

// Nada pendente é o caso normal de todo boot depois do primeiro.
func TestPendingMigrationsTudoAplicado(t *testing.T) {
	fsys := fstest.MapFS{"V001_baseline.sql": {Data: []byte("SELECT 1;")}}

	pend, err := pendingMigrations(fsys, map[string]bool{"V001": true})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(pend) != 0 {
		t.Errorf("quero nenhuma pendente, tenho %d", len(pend))
	}
}

// Arquivo fora do padrão VNNN_ não é migração e não pode virar uma.
func TestPendingMigrationsIgnoraNomeForaDoPadrao(t *testing.T) {
	fsys := fstest.MapFS{
		"init.sql":        {Data: []byte("SELECT 0;")},
		"V010_decima.sql": {Data: []byte("SELECT 10;")},
	}

	pend, err := pendingMigrations(fsys, map[string]bool{})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(pend) != 1 || pend[0].Version != "V010" {
		t.Errorf("quero só V010, tenho %+v", pend)
	}
}

// Sobre um banco que já tem estrutura mas nenhuma linha em
// schema_migrations, V001 é registrada como baseline (o validator já a
// executou) e só a V002 roda de fato.
func TestApplyMigrationsRegistraBaselineEAplicaResto(t *testing.T) {
	fsys := fstest.MapFS{
		"V001_baseline.sql": {Data: []byte("CREATE TABLE baseline();")},
		"V002_nova.sql":     {Data: []byte("ALTER TABLE x ADD COLUMN y INT;")},
	}

	db := &dbFalso{tx: &txFalsa{}}
	// Nenhuma versão aplicada ainda: a consulta de versões devolve vazio,
	// e o INSERT de baseline é o que marca V001.
	db.queryRows = &rowsFalsas{}

	if err := ApplyMigrations(context.Background(), db, fsys); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	juntos := strings.Join(db.execs, "\n")
	if !strings.Contains(juntos, "service.schema_migrations") {
		t.Error("quero a criação da tabela de controle")
	}
	if !strings.Contains(juntos, "'V001'") {
		t.Error("quero V001 registrada como baseline")
	}

	txJuntas := strings.Join(db.tx.execs, "\n")
	if !strings.Contains(txJuntas, "ALTER TABLE x ADD COLUMN y INT;") {
		t.Errorf("quero a V002 executada na transação, tenho: %s", txJuntas)
	}
	if strings.Contains(txJuntas, "CREATE TABLE baseline();") {
		t.Error("V001 não pode ser executada pelo runner — é do validator")
	}
	if !db.tx.comitou {
		t.Error("quero commit da transação")
	}
}

// Segundo boot: tudo já aplicado, nenhuma transação é aberta.
func TestApplyMigrationsIdempotente(t *testing.T) {
	fsys := fstest.MapFS{"V002_nova.sql": {Data: []byte("ALTER TABLE x ADD COLUMN y INT;")}}

	db := &dbFalso{tx: &txFalsa{}}
	db.queryRows = &rowsFalsas{linhas: [][]interface{}{{"V001"}, {"V002"}}}

	if err := ApplyMigrations(context.Background(), db, fsys); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(db.tx.execs) != 0 {
		t.Errorf("quero nenhuma migração executada, tenho: %v", db.tx.execs)
	}
}
