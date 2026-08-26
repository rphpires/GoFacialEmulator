package database

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"regexp"
	"sort"
)

// migration é um arquivo VNNN_*.sql embutido no binário.
type migration struct {
	Version string // "V002"
	Name    string // "V002_manual_emulators.sql"
	SQL     string
}

// nomeDeMigracao casa VNNN_qualquercoisa.sql. Qualquer outro arquivo na
// pasta é ignorado — a pasta também guarda o baseline e pode um dia
// guardar um README.
var nomeDeMigracao = regexp.MustCompile(`^(V\d{3})_.+\.sql$`)

// pendingMigrations devolve, em ordem de versão, as migrações do FS que
// ainda não constam em applied.
func pendingMigrations(fsys fs.FS, applied map[string]bool) ([]migration, error) {
	entradas, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("erro ao ler migrações embutidas: %w", err)
	}

	var pend []migration
	for _, e := range entradas {
		if e.IsDir() {
			continue
		}
		m := nomeDeMigracao.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		versao := m[1]
		if applied[versao] {
			continue
		}
		conteudo, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("erro ao ler %s: %w", e.Name(), err)
		}
		pend = append(pend, migration{Version: versao, Name: e.Name(), SQL: string(conteudo)})
	}

	sort.Slice(pend, func(i, j int) bool { return pend[i].Version < pend[j].Version })
	return pend, nil
}

// versaoBaseline é a migração que o validator executa por conta própria
// (assets/migrations/V001_create_emulator_schema.sql). O runner nunca a
// executa; só a registra, para não tentar recriar um schema que já existe.
const versaoBaseline = "V001"

// ApplyMigrations aplica, em ordem, toda migração embutida ainda não
// registrada em service.schema_migrations. Cada uma roda na sua própria
// transação: um banco meio migrado é pior que uma migração que falhou
// inteira.
//
// Roda depois de ValidateDatabaseOnStartup, que é quem cria o baseline.
// Se o validator tiver recriado os schemas, service.schema_migrations vai
// junto no DROP CASCADE e tudo é reaplicado do zero — as migrações são
// escritas de forma idempotente exatamente por causa disso.
func ApplyMigrations(ctx context.Context, db DBInterface, fsys fs.FS) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS service.schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`)
	if err != nil {
		return fmt.Errorf("erro ao criar service.schema_migrations: %w", err)
	}

	_, err = db.Exec(ctx,
		`INSERT INTO service.schema_migrations (version) VALUES ('`+versaoBaseline+`')
		 ON CONFLICT (version) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("erro ao registrar baseline: %w", err)
	}

	aplicadas := map[string]bool{versaoBaseline: true}
	rows, err := db.Query(ctx, "SELECT version FROM service.schema_migrations")
	if err != nil {
		return fmt.Errorf("erro ao ler migrações aplicadas: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("erro ao ler versão aplicada: %w", err)
		}
		aplicadas[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("erro ao iterar migrações aplicadas: %w", err)
	}

	pend, err := pendingMigrations(fsys, aplicadas)
	if err != nil {
		return err
	}

	for _, mig := range pend {
		log.Printf("🔧 Aplicando migração %s", mig.Name)

		tx, err := db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("erro ao abrir transação para %s: %w", mig.Name, err)
		}

		if _, err := tx.Exec(ctx, mig.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("erro ao aplicar %s: %w", mig.Name, err)
		}

		if _, err := tx.Exec(ctx,
			"INSERT INTO service.schema_migrations (version) VALUES ($1)", mig.Version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("erro ao registrar %s: %w", mig.Name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("erro ao confirmar %s: %w", mig.Name, err)
		}

		log.Printf("✅ Migração %s aplicada", mig.Version)
	}

	return nil
}
