# Emuladores independentes do Invenzi — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fazer o serviço ser dono dos seus emuladores — CRUD por API e por UI, criação em lote por range de portas — com o vínculo ao Invenzi W-Access virando uma opção ligável e desligável.

**Architecture:** Uma coluna `source` em `service.devices` separa o que veio do W-Access do que foi cadastrado à mão; o sync só toca no primeiro. IDs manuais saem de uma sequence começando em 900000, o que preserva `local_controller_id` como PK e evita migrar as FKs soltas de oito tabelas. A validação e a expansão de range vivem em funções puras, testáveis sem banco; o SQL fica numa camada fina por cima.

**Tech Stack:** Go 1.21+, Gin, pgx/v4 (**não** v5), PostgreSQL, `html/template`, JS sem framework (`assets/web/static/js/`), testes com `testing` da stdlib + `net/http/httptest`.

**Spec:** `docs/superpowers/specs/2026-08-26-emulador-crud-independente-design.md`

## Global Constraints

- **Nunca adicionar `source`, `sync_enabled` ou `schema_migrations` à lista `criticalColumns`/`expectedTables` de `internal/database/validator.go`.** Esse validator faz `DROP SCHEMA emulator, service CASCADE` quando acha que falta estrutura. Ensiná-lo sobre as colunas novas apagaria o banco de toda instalação existente no primeiro boot.
- Driver: `github.com/jackc/pgx/v4` e `github.com/jackc/pgconn`. Não introduzir pgx/v5.
- `emulator.device_settings` é chave/valor `(device_id, cfg_id, value)`. A linha `device_id = 0` guarda os padrões globais semeados na V001 e **nunca** pode ser apagada por purga de dispositivo.
- Modelos suportados, exatamente estas strings: `"Hikvision"` e `"Dahua"`. São os únicos que `createEmulator()` sabe instanciar.
- Emuladores fazem bind em `0.0.0.0:{porta}`. `ip_address` é metadado devolvido nas respostas ISAPI/CGI, nunca chega a um `net.Listen`.
- `config.EmulatorDB = config.ServiceDB` — é o mesmo banco físico, dois pools. Uma transação aberta em `ServiceDB` alcança os schemas `service` e `emulator`.
- Sem autenticação em nenhuma rota nova, coerente com o resto do serviço.
- Teto de 1000 portas por lote, que é a largura do range publicado no `docker-compose.yml` (4000-4999).
- Textos de UI e mensagens de erro de API em português; identificadores de código e chaves JSON em inglês, como o repositório já faz.
- Rodar `go build ./... && go vet ./... && go test ./...` antes de cada commit.

---

### Task 1: Runner de migração e V002

O repositório não tem migration runner. `ValidateDatabaseOnStartup()` confere estrutura e recria por `DROP SCHEMA` quando falta algo — inútil para adicionar coluna sem perder dados. Esta tarefa cria o mecanismo que todas as próximas assumem.

**Files:**
- Create: `assets/migrations/V002_manual_emulators.sql`
- Create: `internal/database/migrate.go`
- Create: `internal/database/migrate_test.go`
- Create: `internal/database/fakes_test.go`
- Modify: `assets/assets.go`
- Modify: `cmd/emulator-service/main.go:37-46`

**Interfaces:**
- Consumes: `database.DBInterface` (`internal/database/connection.go:11`), `assets.MigrationSQL()`
- Produces: `assets.MigrationFiles() (fs.FS, error)`; `database.ApplyMigrations(ctx context.Context, db DBInterface, fsys fs.FS) error`; `database.pendingMigrations(fsys fs.FS, applied map[string]bool) ([]migration, error)`; tipo não exportado `migration{Version, Name, SQL string}`

- [ ] **Step 1: Escrever o teste de seleção de migrações pendentes**

Criar `internal/database/migrate_test.go`:

```go
package database

import (
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
```

- [ ] **Step 2: Rodar o teste e ver falhar**

Run: `go test ./internal/database/ -run TestPendingMigrations -v`
Expected: FAIL — `undefined: pendingMigrations`

- [ ] **Step 3: Implementar `pendingMigrations`**

Criar `internal/database/migrate.go`:

```go
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
```

- [ ] **Step 4: Rodar o teste e ver passar**

Run: `go test ./internal/database/ -run TestPendingMigrations -v`
Expected: PASS (3 testes)

- [ ] **Step 5: Escrever os fakes de banco do pacote `database`**

Criar `internal/database/fakes_test.go`. Os testes deste repositório não usam PostgreSQL de verdade; o padrão é um fake que implementa a interface (veja `internal/handlers/device_mode_test.go`).

```go
package database

import (
	"context"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v4"
)

// linhaFalsa devolve valores fixos (ou um erro) no Scan.
type linhaFalsa struct {
	valores []interface{}
	err     error
}

func (l linhaFalsa) Scan(dest ...interface{}) error {
	if l.err != nil {
		return l.err
	}
	for i := range dest {
		if i >= len(l.valores) {
			break
		}
		switch d := dest[i].(type) {
		case *bool:
			*d = l.valores[i].(bool)
		case *int:
			*d = l.valores[i].(int)
		case *string:
			*d = l.valores[i].(string)
		}
	}
	return nil
}

// rowsFalsas implementa pgx.Rows sobre uma matriz de valores.
type rowsFalsas struct {
	linhas [][]interface{}
	pos    int
	err    error
}

func (r *rowsFalsas) Close()                                       {}
func (r *rowsFalsas) Err() error                                   { return r.err }
func (r *rowsFalsas) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *rowsFalsas) FieldDescriptions() []pgproto3.FieldDescription { return nil }
func (r *rowsFalsas) Values() ([]interface{}, error)               { return nil, nil }
func (r *rowsFalsas) RawValues() [][]byte                          { return nil }

func (r *rowsFalsas) Next() bool {
	if r.pos >= len(r.linhas) {
		return false
	}
	r.pos++
	return true
}

func (r *rowsFalsas) Scan(dest ...interface{}) error {
	linha := r.linhas[r.pos-1]
	return linhaFalsa{valores: linha}.Scan(dest...)
}

// txFalsa implementa pgx.Tx registrando o que foi executado.
type txFalsa struct {
	execs      []string
	execArgs   [][]interface{}
	comitou    bool
	desfez     bool
	execErr    error
	queryRows  pgx.Rows
	queryRowFn func(sql string) pgx.Row
}

func (t *txFalsa) Begin(ctx context.Context) (pgx.Tx, error) { return t, nil }
func (t *txFalsa) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error { return f(t) }
func (t *txFalsa) Commit(ctx context.Context) error   { t.comitou = true; return nil }
func (t *txFalsa) Rollback(ctx context.Context) error { t.desfez = true; return nil }
func (t *txFalsa) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *txFalsa) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (t *txFalsa) LargeObjects() pgx.LargeObjects                              { return pgx.LargeObjects{} }
func (t *txFalsa) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *txFalsa) Conn() *pgx.Conn { return nil }
func (t *txFalsa) QueryFunc(ctx context.Context, sql string, args []interface{}, scans []interface{}, f func(pgx.QueryFuncRow) error) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (t *txFalsa) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	t.execs = append(t.execs, sql)
	t.execArgs = append(t.execArgs, args)
	return pgconn.CommandTag{}, t.execErr
}

func (t *txFalsa) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if t.queryRows == nil {
		return &rowsFalsas{}, nil
	}
	return t.queryRows, nil
}

func (t *txFalsa) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if t.queryRowFn != nil {
		return t.queryRowFn(sql)
	}
	return linhaFalsa{}
}

// dbFalso implementa DBInterface devolvendo sempre a mesma txFalsa.
type dbFalso struct {
	tx        *txFalsa
	execs     []string
	queryRows pgx.Rows
	linha     pgx.Row
}

func (d *dbFalso) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if d.queryRows == nil {
		return &rowsFalsas{}, nil
	}
	return d.queryRows, nil
}

func (d *dbFalso) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if d.linha == nil {
		return linhaFalsa{}
	}
	return d.linha
}

func (d *dbFalso) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	d.execs = append(d.execs, sql)
	return pgconn.CommandTag{}, nil
}

func (d *dbFalso) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.tx == nil {
		d.tx = &txFalsa{}
	}
	return d.tx, nil
}

func (d *dbFalso) Ping(ctx context.Context) error { return nil }
```

- [ ] **Step 6: Escrever o teste de `ApplyMigrations`**

Acrescentar em `internal/database/migrate_test.go`:

```go
import (
	"context"
	"strings"
)

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
```

- [ ] **Step 7: Rodar o teste e ver falhar**

Run: `go test ./internal/database/ -run TestApplyMigrations -v`
Expected: FAIL — `undefined: ApplyMigrations`

- [ ] **Step 8: Implementar `ApplyMigrations`**

Acrescentar em `internal/database/migrate.go`:

```go
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
```

- [ ] **Step 9: Rodar o teste e ver passar**

Run: `go test ./internal/database/ -v`
Expected: PASS

- [ ] **Step 10: Escrever a migração V002**

Criar `assets/migrations/V002_manual_emulators.sql`:

```sql
-- V002 — emuladores cadastrados fora do W-Access.
--
-- Todo DDL aqui é idempotente de propósito: o validator recria os schemas
-- por DROP CASCADE quando acha estrutura faltando, e nesse caso esta
-- migração é reaplicada do zero.

-- Origem do dispositivo. O DEFAULT é o backfill: toda linha que já existe
-- veio do W-Access, então uma instalação que sincroniza hoje continua se
-- comportando igual depois da atualização.
ALTER TABLE service.devices
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'wxs';

ALTER TABLE service.devices
    DROP CONSTRAINT IF EXISTS devices_source_check;
ALTER TABLE service.devices
    ADD CONSTRAINT devices_source_check CHECK (source IN ('wxs', 'manual'));

CREATE INDEX IF NOT EXISTS idx_devices_source ON service.devices(source);

-- IDs manuais começam acima de qualquer LocalControllerID plausível de um
-- W-Access real. Colisão viola a PK e derruba a transação inteira — falha
-- barulhenta, não corrupção silenciosa.
CREATE SEQUENCE IF NOT EXISTS service.manual_device_id_seq START 900000;

-- O vínculo com o Invenzi vira opção. Sem linha em wxs_settings o sync é
-- considerado desligado, que já é o estado de fato de quem nunca
-- configurou o W-Access.
ALTER TABLE service.wxs_settings
    ADD COLUMN IF NOT EXISTS sync_enabled BOOLEAN NOT NULL DEFAULT TRUE;
```

- [ ] **Step 11: Expor a pasta de migrações pelo `assets`**

Modificar `assets/assets.go`, acrescentando depois de `MigrationSQL`:

```go
// MigrationFiles devolve o subsistema "migrations" para o runner de
// migração (internal/database/migrate.go). MigrationSQL continua servindo
// o validator, que só conhece o baseline.
func MigrationFiles() (fs.FS, error) {
	return fs.Sub(root, "migrations")
}
```

- [ ] **Step 12: Chamar o runner na subida**

Modificar `cmd/emulator-service/main.go`. Depois do bloco que obtém `serviceDB` (`serviceDB, err := database.GetServiceDB(...)`), antes de `GetEmulatorDB`, inserir:

```go
	tracer.Info("Applying schema migrations...")
	migFS, err := assets.MigrationFiles()
	if err != nil {
		log.Fatalf("Failed to open embedded migrations: %v", err)
	}
	migCtx, migCancel := context.WithTimeout(context.Background(), 60*time.Second)
	err = database.ApplyMigrations(migCtx, serviceDB, migFS)
	migCancel()
	if err != nil {
		// Banco meio migrado é pior que serviço que não sobe.
		log.Fatalf("Failed to apply migrations: %v", err)
	}
	tracer.Info("Schema migrations up to date")
```

E acrescentar `"GoFacialEmulator/assets"` ao bloco de imports.

- [ ] **Step 13: Compilar e rodar a suíte**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS, sem erro de compilação

- [ ] **Step 14: Commit**

```bash
git add assets/assets.go assets/migrations/V002_manual_emulators.sql internal/database/migrate.go internal/database/migrate_test.go internal/database/fakes_test.go cmd/emulator-service/main.go
git commit -m "feat(db): runner de migração e V002 com source, sequence e sync_enabled"
```

---

### Task 2: Coluna `source` no modelo e nas leituras

Sem isso, nada distingue um emulador manual de um vindo do W-Access, e o cleanup da Task 4 não teria por onde filtrar.

**Files:**
- Modify: `internal/models/models.go:4-17`
- Modify: `internal/emulator/manager.go:213-237` (`upsertDevice`), `:276-326` (`ListDevices`), `:327-380` (`GetDevice`), `:381-412` (`getDeviceUnsafe`), `:920-999` (`ListDevicesWithFilters`)
- Modify: `internal/handlers/handlers.go:836-893` (`getCurrentDevicesWithFilters`)
- Create: `internal/emulator/fakes_test.go`
- Create: `internal/emulator/source_test.go`

**Interfaces:**
- Consumes: `database.DBInterface`
- Produces: campo `models.Device.Source string` com tag `json:"source"`; chave `"source"` no mapa que os templates consomem

- [ ] **Step 1: Escrever os fakes do pacote `emulator`**

Criar `internal/emulator/fakes_test.go`. Mesmo conteúdo dos fakes da Task 1, com o pacote trocado — os dois pacotes precisam dos seus próprios porque fakes de teste não atravessam a fronteira de pacote.

```go
package emulator

import (
	"context"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v4"
)

type linhaFalsa struct {
	valores []interface{}
	err     error
}

func (l linhaFalsa) Scan(dest ...interface{}) error {
	if l.err != nil {
		return l.err
	}
	for i := range dest {
		if i >= len(l.valores) {
			break
		}
		switch d := dest[i].(type) {
		case *bool:
			*d = l.valores[i].(bool)
		case *int:
			*d = l.valores[i].(int)
		case *string:
			*d = l.valores[i].(string)
		}
	}
	return nil
}

type rowsFalsas struct {
	linhas [][]interface{}
	pos    int
	err    error
}

func (r *rowsFalsas) Close()                                         {}
func (r *rowsFalsas) Err() error                                     { return r.err }
func (r *rowsFalsas) CommandTag() pgconn.CommandTag                  { return pgconn.CommandTag{} }
func (r *rowsFalsas) FieldDescriptions() []pgproto3.FieldDescription { return nil }
func (r *rowsFalsas) Values() ([]interface{}, error)                 { return nil, nil }
func (r *rowsFalsas) RawValues() [][]byte                            { return nil }

func (r *rowsFalsas) Next() bool {
	if r.pos >= len(r.linhas) {
		return false
	}
	r.pos++
	return true
}

func (r *rowsFalsas) Scan(dest ...interface{}) error {
	return linhaFalsa{valores: r.linhas[r.pos-1]}.Scan(dest...)
}

type txFalsa struct {
	execs      []string
	execArgs   [][]interface{}
	comitou    bool
	desfez     bool
	execErr    error
	queryRows  pgx.Rows
	queryRowFn func(sql string) pgx.Row
}

func (t *txFalsa) Begin(ctx context.Context) (pgx.Tx, error)                 { return t, nil }
func (t *txFalsa) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error { return f(t) }
func (t *txFalsa) Commit(ctx context.Context) error                          { t.comitou = true; return nil }
func (t *txFalsa) Rollback(ctx context.Context) error                        { t.desfez = true; return nil }
func (t *txFalsa) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *txFalsa) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (t *txFalsa) LargeObjects() pgx.LargeObjects                              { return pgx.LargeObjects{} }
func (t *txFalsa) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *txFalsa) Conn() *pgx.Conn { return nil }
func (t *txFalsa) QueryFunc(ctx context.Context, sql string, args []interface{}, scans []interface{}, f func(pgx.QueryFuncRow) error) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (t *txFalsa) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	t.execs = append(t.execs, sql)
	t.execArgs = append(t.execArgs, args)
	return pgconn.CommandTag{}, t.execErr
}

func (t *txFalsa) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if t.queryRows == nil {
		return &rowsFalsas{}, nil
	}
	return t.queryRows, nil
}

func (t *txFalsa) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if t.queryRowFn != nil {
		return t.queryRowFn(sql)
	}
	return linhaFalsa{}
}

type dbFalso struct {
	tx         *txFalsa
	execs      []string
	execArgs   [][]interface{}
	queryRows  pgx.Rows
	linha      pgx.Row
}

func (d *dbFalso) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if d.queryRows == nil {
		return &rowsFalsas{}, nil
	}
	return d.queryRows, nil
}

func (d *dbFalso) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if d.linha == nil {
		return linhaFalsa{}
	}
	return d.linha
}

func (d *dbFalso) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	d.execs = append(d.execs, sql)
	d.execArgs = append(d.execArgs, args)
	return pgconn.CommandTag{}, nil
}

func (d *dbFalso) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.tx == nil {
		d.tx = &txFalsa{}
	}
	return d.tx, nil
}

func (d *dbFalso) Ping(ctx context.Context) error { return nil }
```

- [ ] **Step 2: Escrever o teste de leitura da coluna**

Criar `internal/emulator/source_test.go`:

```go
package emulator

import (
	"testing"

	"GoFacialEmulator/internal/trace"
)

// A ordem das colunas no SELECT e no Scan precisa bater. Um teste que
// devolve uma linha completa é o que pega uma coluna acrescentada no SQL
// e esquecida no Scan — o erro mais fácil de cometer aqui.
func TestListDevicesWithFiltersLeSource(t *testing.T) {
	db := &dbFalso{
		queryRows: &rowsFalsas{linhas: [][]interface{}{
			// local_controller_id, name, ip_address, port, model, status,
			// enabled, event_interval, total_users, log_enabled, type, source
			{900001, "lab-4000", "192.168.1.50", 4000, "Dahua", "stopped", 1, 10, 0, 0, 1, "manual"},
			{17, "Portaria", "10.0.0.7", 7070, "Hikvision", "stopped", 1, 10, 3, 0, 2, "wxs"},
		}},
	}

	m := &Manager{ServiceDB: db, Tracer: trace.NewTracer(), emulators: map[int]Emulator{}}

	devices, err := m.ListDevicesWithFilters(map[string]string{})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("quero 2 dispositivos, tenho %d", len(devices))
	}
	if devices[0].Source != SourceManual {
		t.Errorf("dispositivo 900001: source %q, quero %q", devices[0].Source, SourceManual)
	}
	if devices[1].Source != SourceWXS {
		t.Errorf("dispositivo 17: source %q, quero %q", devices[1].Source, SourceWXS)
	}
}
```

Este teste referencia `SourceManual`/`SourceWXS`, definidos na Task 3. Para não acoplar as tarefas, declare as duas constantes já nesta tarefa, em `internal/emulator/manager.go`, logo abaixo dos imports:

```go
// Origem de um dispositivo em service.devices.
const (
	SourceWXS    = "wxs"
	SourceManual = "manual"
)
```

- [ ] **Step 3: Rodar o teste e ver falhar**

Run: `go test ./internal/emulator/ -run TestListDevicesWithFiltersLeSource -v`
Expected: FAIL — `device.Source undefined`

- [ ] **Step 4: Adicionar o campo ao modelo**

Modificar `internal/models/models.go`, no struct `Device`, depois de `LogEnabled`:

```go
	// Source separa o que veio do W-Access ("wxs") do que foi cadastrado
	// pela API ou pela tela ("manual"). O sync só apaga o primeiro.
	Source string `json:"source"`
```

- [ ] **Step 5: Ler `source` nas quatro consultas do manager**

Em `internal/emulator/manager.go`:

`ListDevices` (linha ~281) — SQL e Scan:

```go
	query := `
		SELECT local_controller_id, name, ip_address, port, model, enabled, type,
		       status, event_interval, total_users, log_enabled, source
		FROM service.devices
		ORDER BY local_controller_id
	`
```
No `rows.Scan` correspondente, acrescentar `&device.Source` como último destino.

`GetDevice` (linha ~332) e `getDeviceUnsafe` (linha ~386) — mesma coluna `source` ao fim do SELECT e `&device.Source` ao fim do `Scan`.

`ListDevicesWithFilters` (linha ~929):

```go
	query := `
		SELECT local_controller_id, name, ip_address, port, model, status, enabled,
		       event_interval, total_users, log_enabled, type, source
		FROM service.devices
		WHERE 1=1
	`
```
E no `rows.Scan`, `&device.Source` depois de `&device.Type`.

- [ ] **Step 6: Gravar `source` no upsert do sync**

Em `upsertDevice` (linha ~213), o INSERT ganha a coluna e o `DO UPDATE` **não** a toca — reescrever a origem de um dispositivo existente seria mudar quem manda nele:

```go
	query := `
		INSERT INTO service.devices (
			local_controller_id, name, ip_address, port, model, enabled, type,
			status, event_interval, total_users, log_enabled, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'wxs')
		ON CONFLICT (local_controller_id) DO UPDATE SET
			name = EXCLUDED.name,
			ip_address = EXCLUDED.ip_address,
			port = EXCLUDED.port,
			model = EXCLUDED.model,
			enabled = EXCLUDED.enabled,
			type = EXCLUDED.type,
			event_interval = EXCLUDED.event_interval,
			updated_at = NOW()
	`
```

- [ ] **Step 7: Levar `source` até o template**

Em `internal/handlers/handlers.go`, dentro do `append` de `getCurrentDevicesWithFilters` (linha ~878), acrescentar ao mapa:

```go
			"source":      device.Source,
```

- [ ] **Step 8: Rodar os testes**

Run: `go test ./internal/emulator/ ./internal/handlers/ -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/models/models.go internal/emulator/manager.go internal/emulator/fakes_test.go internal/emulator/source_test.go internal/handlers/handlers.go
git commit -m "feat(devices): ler e gravar a origem do dispositivo"
```

---

### Task 3: Validação e expansão de range (funções puras)

Toda a regra de negócio do cadastro em lote — defaults, limites, nomes gerados, detecção de conflito — sem uma linha de SQL. É a parte com mais casos de borda e a que dá para cobrir de verdade nesta base, que não tem PostgreSQL nos testes.

**Files:**
- Create: `internal/emulator/spec.go`
- Create: `internal/emulator/spec_test.go`

**Interfaces:**
- Consumes: nada além da stdlib
- Produces: `DeviceSpec`, `RangeSpec`, `ConflictError`; `ErrSyncDisabled`, `ErrDeviceIsManaged`, `ErrDeviceRunning`, `ErrInvalidSpec`, `ErrDeviceNotFound`; constantes `ModelHikvision`, `ModelDahua`, `MaxRangeSize`, `IPPadrao`, `IntervaloPadrao`; métodos `(*DeviceSpec).Normalize() error`, `(*RangeSpec).Normalize() error`, `(RangeSpec).Expand() []DeviceSpec`; função `Conflicts(desejadas []int, ocupadas map[int]bool) []int`

- [ ] **Step 1: Escrever os testes**

Criar `internal/emulator/spec_test.go`:

```go
package emulator

import (
	"errors"
	"reflect"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestDeviceSpecNormalizeAplicaPadroes(t *testing.T) {
	s := DeviceSpec{Name: "lab-01", Model: ModelDahua, Port: 4000}

	if err := s.Normalize(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if s.IPAddress != IPPadrao {
		t.Errorf("ip: %q, quero %q", s.IPAddress, IPPadrao)
	}
	if s.EventInterval != IntervaloPadrao {
		t.Errorf("intervalo: %d, quero %d", s.EventInterval, IntervaloPadrao)
	}
	if s.Enabled == nil || !*s.Enabled {
		t.Error("enabled: quero true por padrão")
	}
	if s.AutoStart {
		t.Error("auto_start: quero false por padrão")
	}
}

func TestDeviceSpecNormalizeRespeitaEnabledFalse(t *testing.T) {
	s := DeviceSpec{Name: "lab-01", Model: ModelDahua, Port: 4000, Enabled: boolPtr(false)}

	if err := s.Normalize(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if *s.Enabled {
		t.Error("enabled false explícito virou true")
	}
}

func TestDeviceSpecNormalizeRejeita(t *testing.T) {
	casos := []struct {
		nome string
		spec DeviceSpec
	}{
		{"nome vazio", DeviceSpec{Model: ModelDahua, Port: 4000}},
		{"nome só espaços", DeviceSpec{Name: "   ", Model: ModelDahua, Port: 4000}},
		{"modelo desconhecido", DeviceSpec{Name: "x", Model: "Intelbras", Port: 4000}},
		{"modelo vazio", DeviceSpec{Name: "x", Port: 4000}},
		{"porta zero", DeviceSpec{Name: "x", Model: ModelDahua, Port: 0}},
		{"porta negativa", DeviceSpec{Name: "x", Model: ModelDahua, Port: -1}},
		{"porta acima de 65535", DeviceSpec{Name: "x", Model: ModelDahua, Port: 65536}},
		{"intervalo negativo", DeviceSpec{Name: "x", Model: ModelDahua, Port: 4000, EventInterval: -5}},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := c.spec
			err := s.Normalize()
			if !errors.Is(err, ErrInvalidSpec) {
				t.Errorf("quero ErrInvalidSpec, tenho %v", err)
			}
		})
	}
}

func TestDeviceSpecNormalizeAceitaOsDoisModelos(t *testing.T) {
	for _, modelo := range []string{ModelDahua, ModelHikvision} {
		s := DeviceSpec{Name: "x", Model: modelo, Port: 4000}
		if err := s.Normalize(); err != nil {
			t.Errorf("modelo %q rejeitado: %v", modelo, err)
		}
	}
}

func TestRangeSpecExpandGeraUmPorPorta(t *testing.T) {
	r := RangeSpec{
		NamePrefix: "lab",
		Model:      ModelHikvision,
		IPAddress:  "192.168.1.50",
		PortStart:  4000,
		PortEnd:    4002,
	}
	if err := r.Normalize(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	specs := r.Expand()

	if len(specs) != 3 {
		t.Fatalf("quero 3 specs, tenho %d", len(specs))
	}
	querNomes := []string{"lab-4000", "lab-4001", "lab-4002"}
	for i, s := range specs {
		if s.Name != querNomes[i] {
			t.Errorf("spec %d: nome %q, quero %q", i, s.Name, querNomes[i])
		}
		if s.Port != 4000+i {
			t.Errorf("spec %d: porta %d, quero %d", i, s.Port, 4000+i)
		}
		if s.Model != ModelHikvision || s.IPAddress != "192.168.1.50" {
			t.Errorf("spec %d: modelo/ip não vieram do lote: %+v", i, s)
		}
	}
}

func TestRangeSpecExpandPortaUnica(t *testing.T) {
	r := RangeSpec{NamePrefix: "solo", Model: ModelDahua, PortStart: 4000, PortEnd: 4000}
	if err := r.Normalize(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if specs := r.Expand(); len(specs) != 1 {
		t.Errorf("quero 1 spec, tenho %d", len(specs))
	}
}

func TestRangeSpecNormalizeRejeita(t *testing.T) {
	casos := []struct {
		nome string
		spec RangeSpec
	}{
		{"prefixo vazio", RangeSpec{Model: ModelDahua, PortStart: 4000, PortEnd: 4001}},
		{"modelo inválido", RangeSpec{NamePrefix: "x", Model: "Nedap", PortStart: 4000, PortEnd: 4001}},
		{"fim antes do início", RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 4010, PortEnd: 4000}},
		{"início zero", RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 0, PortEnd: 4000}},
		{"fim acima de 65535", RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 4000, PortEnd: 70000}},
		{"lote maior que o teto", RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 1000, PortEnd: 1000 + MaxRangeSize}},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := c.spec
			if err := s.Normalize(); !errors.Is(err, ErrInvalidSpec) {
				t.Errorf("quero ErrInvalidSpec, tenho %v", err)
			}
		})
	}
}

func TestRangeSpecNormalizeAceitaOTetoExato(t *testing.T) {
	r := RangeSpec{NamePrefix: "x", Model: ModelDahua, PortStart: 1000, PortEnd: 1000 + MaxRangeSize - 1}
	if err := r.Normalize(); err != nil {
		t.Errorf("lote de exatamente %d portas rejeitado: %v", MaxRangeSize, err)
	}
}

func TestConflictsDevolveOrdenadoESemRepeticao(t *testing.T) {
	ocupadas := map[int]bool{4003: true, 4001: true, 8080: true}

	got := Conflicts([]int{4000, 4001, 4001, 4003, 4004, 8080}, ocupadas)

	quero := []int{4001, 4003, 8080}
	if !reflect.DeepEqual(got, quero) {
		t.Errorf("conflitos: %v, quero %v", got, quero)
	}
}

func TestConflictsSemColisaoDevolveVazio(t *testing.T) {
	if got := Conflicts([]int{4000, 4001}, map[int]bool{9000: true}); len(got) != 0 {
		t.Errorf("quero nenhum conflito, tenho %v", got)
	}
}

func TestConflictErrorListaAsPortas(t *testing.T) {
	err := &ConflictError{Ports: []int{4001, 4003}}
	if !errors.Is(err, ErrInvalidSpec) {
		t.Error("ConflictError precisa casar com ErrInvalidSpec para virar 400 no handler")
	}
	if msg := err.Error(); msg == "" {
		t.Error("mensagem vazia")
	}
}

func TestConflictErrorMencionaPortaReservada(t *testing.T) {
	err := &ConflictError{Reserved: []int{8080}}
	if msg := err.Error(); msg == "" {
		t.Error("mensagem vazia")
	}
}
```

- [ ] **Step 2: Rodar os testes e ver falhar**

Run: `go test ./internal/emulator/ -run 'TestDeviceSpec|TestRangeSpec|TestConflict' -v`
Expected: FAIL — `undefined: DeviceSpec`

- [ ] **Step 3: Implementar `spec.go`**

Criar `internal/emulator/spec.go`:

```go
package emulator

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Modelos que createEmulator() sabe instanciar. Qualquer outro valor
// cadastraria um dispositivo que nunca conseguiria iniciar.
const (
	ModelHikvision = "Hikvision"
	ModelDahua     = "Dahua"
)

const (
	// MaxRangeSize é a largura do range publicado no docker-compose.yml
	// (4000-4999). Um lote maior que isso cadastra emuladores que, sob
	// Docker, nascem inalcançáveis.
	MaxRangeSize = 1000

	// IPPadrao e IntervaloPadrao valem quando o payload omite os campos.
	IPPadrao        = "127.0.0.1"
	IntervaloPadrao = 10
)

var (
	// ErrInvalidSpec é a raiz de tudo que o cliente errou no payload —
	// o handler converte em 400. ConflictError também casa com ele.
	ErrInvalidSpec = errors.New("dados inválidos")

	// ErrSyncDisabled: o vínculo com o W-Access está desligado, ou nunca
	// foi configurado. Não é falha, é estado — 409, não 500.
	ErrSyncDisabled = errors.New("sincronização com o W-Access desligada")

	// ErrDeviceIsManaged: a verdade do dispositivo mora no W-Access, e o
	// próximo sync sobrescreveria a edição em silêncio.
	ErrDeviceIsManaged = errors.New("dispositivo é gerenciado pelo W-Access")

	// ErrDeviceRunning: o emulador guarda uma cópia de models.Device em
	// memória; editar a quente deixaria as respostas ISAPI/CGI mentindo.
	ErrDeviceRunning = errors.New("emulador está rodando")

	ErrDeviceNotFound = errors.New("dispositivo não encontrado")
)

// DeviceSpec é o corpo de POST /api/emulators e de PUT /api/emulators/:id.
// Enabled é ponteiro para distinguir "não informado" (vira true) de
// "informado como false".
type DeviceSpec struct {
	Name          string `json:"name"`
	Model         string `json:"model"`
	IPAddress     string `json:"ip_address"`
	Port          int    `json:"port"`
	EventInterval int    `json:"event_interval"`
	Enabled       *bool  `json:"enabled"`
	AutoStart     bool   `json:"auto_start"`
}

// RangeSpec é o corpo de POST /api/emulators/range. Só a porta varia entre
// os itens: o bind é 0.0.0.0, então um range é N portas no mesmo host.
type RangeSpec struct {
	NamePrefix    string `json:"name_prefix"`
	Model         string `json:"model"`
	IPAddress     string `json:"ip_address"`
	PortStart     int    `json:"port_start"`
	PortEnd       int    `json:"port_end"`
	EventInterval int    `json:"event_interval"`
	Enabled       *bool  `json:"enabled"`
	AutoStart     bool   `json:"auto_start"`
}

// ConflictError lista as portas que impediram a criação. Ports são portas
// de outros dispositivos cadastrados; Reserved são portas do próprio
// serviço, que merecem uma frase diferente porque a causa é outra.
type ConflictError struct {
	Ports    []int
	Reserved []int
}

func (e *ConflictError) Error() string {
	var partes []string
	if len(e.Ports) > 0 {
		partes = append(partes, fmt.Sprintf("portas já usadas por outros emuladores: %v", e.Ports))
	}
	if len(e.Reserved) > 0 {
		partes = append(partes,
			fmt.Sprintf("portas reservadas pelo próprio serviço: %v", e.Reserved))
	}
	if len(partes) == 0 {
		return "conflito de portas"
	}
	return strings.Join(partes, "; ")
}

// Is faz ConflictError casar com ErrInvalidSpec, para o handler mapear
// tudo que é erro de payload num 400 só.
func (e *ConflictError) Is(target error) bool { return target == ErrInvalidSpec }

func modeloValido(modelo string) bool {
	return modelo == ModelHikvision || modelo == ModelDahua
}

func portaValida(p int) bool { return p >= 1 && p <= 65535 }

// Normalize preenche os padrões e valida. Chamar antes de qualquer uso.
func (s *DeviceSpec) Normalize() error {
	s.Name = strings.TrimSpace(s.Name)
	s.IPAddress = strings.TrimSpace(s.IPAddress)

	if s.Name == "" {
		return fmt.Errorf("%w: nome é obrigatório", ErrInvalidSpec)
	}
	if !modeloValido(s.Model) {
		return fmt.Errorf("%w: modelo deve ser %q ou %q", ErrInvalidSpec, ModelHikvision, ModelDahua)
	}
	if !portaValida(s.Port) {
		return fmt.Errorf("%w: porta deve estar entre 1 e 65535", ErrInvalidSpec)
	}
	if s.EventInterval < 0 {
		return fmt.Errorf("%w: intervalo de eventos não pode ser negativo", ErrInvalidSpec)
	}

	if s.IPAddress == "" {
		s.IPAddress = IPPadrao
	}
	if s.EventInterval == 0 {
		s.EventInterval = IntervaloPadrao
	}
	if s.Enabled == nil {
		verdadeiro := true
		s.Enabled = &verdadeiro
	}
	return nil
}

// Normalize preenche os padrões do lote e valida os limites do range.
func (s *RangeSpec) Normalize() error {
	s.NamePrefix = strings.TrimSpace(s.NamePrefix)
	s.IPAddress = strings.TrimSpace(s.IPAddress)

	if s.NamePrefix == "" {
		return fmt.Errorf("%w: prefixo de nome é obrigatório", ErrInvalidSpec)
	}
	if !modeloValido(s.Model) {
		return fmt.Errorf("%w: modelo deve ser %q ou %q", ErrInvalidSpec, ModelHikvision, ModelDahua)
	}
	if !portaValida(s.PortStart) || !portaValida(s.PortEnd) {
		return fmt.Errorf("%w: portas devem estar entre 1 e 65535", ErrInvalidSpec)
	}
	if s.PortEnd < s.PortStart {
		return fmt.Errorf("%w: porta final não pode ser menor que a inicial", ErrInvalidSpec)
	}
	if tamanho := s.PortEnd - s.PortStart + 1; tamanho > MaxRangeSize {
		return fmt.Errorf("%w: lote de %d portas excede o máximo de %d",
			ErrInvalidSpec, tamanho, MaxRangeSize)
	}
	if s.EventInterval < 0 {
		return fmt.Errorf("%w: intervalo de eventos não pode ser negativo", ErrInvalidSpec)
	}

	if s.IPAddress == "" {
		s.IPAddress = IPPadrao
	}
	if s.EventInterval == 0 {
		s.EventInterval = IntervaloPadrao
	}
	if s.Enabled == nil {
		verdadeiro := true
		s.Enabled = &verdadeiro
	}
	return nil
}

// Expand converte o lote em um DeviceSpec por porta. O nome carrega a
// porta, e não um índice: a porta é o que o operador procura quando algo
// falha. Chamar só depois de Normalize.
func (s RangeSpec) Expand() []DeviceSpec {
	specs := make([]DeviceSpec, 0, s.PortEnd-s.PortStart+1)
	for porta := s.PortStart; porta <= s.PortEnd; porta++ {
		specs = append(specs, DeviceSpec{
			Name:          fmt.Sprintf("%s-%d", s.NamePrefix, porta),
			Model:         s.Model,
			IPAddress:     s.IPAddress,
			Port:          porta,
			EventInterval: s.EventInterval,
			Enabled:       s.Enabled,
			AutoStart:     s.AutoStart,
		})
	}
	return specs
}

// Conflicts devolve, ordenadas e sem repetição, as portas desejadas que já
// estão ocupadas.
func Conflicts(desejadas []int, ocupadas map[int]bool) []int {
	vistas := map[int]bool{}
	var conflitos []int
	for _, p := range desejadas {
		if ocupadas[p] && !vistas[p] {
			vistas[p] = true
			conflitos = append(conflitos, p)
		}
	}
	sort.Ints(conflitos)
	return conflitos
}
```

Remover as constantes `SourceWXS`/`SourceManual` de `manager.go` (adicionadas na Task 2) e trazê-las para `spec.go`, junto das outras constantes de domínio:

```go
// Origem de um dispositivo em service.devices.
const (
	SourceWXS    = "wxs"
	SourceManual = "manual"
)
```

- [ ] **Step 4: Rodar os testes e ver passar**

Run: `go test ./internal/emulator/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/emulator/spec.go internal/emulator/spec_test.go internal/emulator/manager.go
git commit -m "feat(emulator): validação e expansão de range como funções puras"
```

---

### Task 4: Toggle de sync e cleanup que respeita a origem

Esta é a tarefa que desacopla de fato. Sem ela, o primeiro refresh apagaria todo emulador manual.

**Files:**
- Create: `internal/database/sync_settings.go`
- Create: `internal/database/sync_settings_test.go`
- Create: `internal/emulator/sync_test.go`
- Modify: `internal/emulator/manager.go:118-171` (`RefreshDevices`), `:239-275` (`cleanupOrphanedDevices`)
- Modify: `internal/handlers/handlers.go:213-253` (rotas), `:406-421` (`refreshDevices`), `:513-524` (`apiRefreshEmulators`), `:906-921` (`settingsPage`)
- Create: `internal/handlers/sync_settings.go`
- Create: `internal/handlers/sync_settings_test.go`

**Interfaces:**
- Consumes: `DBInterface`, `ErrSyncDisabled`, `SourceWXS` (Task 3), `models.Device`
- Produces: `database.GetSyncEnabled(ctx, db) (bool, error)`; `database.SetSyncEnabled(ctx, db, bool) error`; `database.ErrNoWxsSettings`; `emulator.orphanIDs(devices []models.Device, validos map[int]bool) []int`; handler `(*Handler).apiSetSyncEnabled`; rota `POST /api/settings/sync`

- [ ] **Step 1: Escrever o teste de leitura do toggle**

Criar `internal/database/sync_settings_test.go`:

```go
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
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/database/ -run TestGetSyncEnabled -v`
Expected: FAIL — `undefined: GetSyncEnabled`

- [ ] **Step 3: Implementar `sync_settings.go`**

Criar `internal/database/sync_settings.go`:

```go
package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
)

// ErrNoWxsSettings: ligar ou desligar o sync exige uma conexão de W-Access
// gravada. Sem ela o toggle não teria sobre o que agir.
var ErrNoWxsSettings = errors.New("nenhuma configuração de W-Access gravada")

// GetSyncEnabled diz se o vínculo com o Invenzi está ligado. Sem linha em
// service.wxs_settings devolve false sem erro: instalação que nunca
// configurou o W-Access não sincroniza, e isso é estado normal, não falha.
func GetSyncEnabled(ctx context.Context, db DBInterface) (bool, error) {
	var ligado bool
	err := db.QueryRow(ctx, `
		SELECT sync_enabled
		FROM service.wxs_settings
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&ligado)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("erro ao ler sync_enabled: %w", err)
	}
	return ligado, nil
}

// SetSyncEnabled grava o toggle na linha mais recente de wxs_settings.
func SetSyncEnabled(ctx context.Context, db DBInterface, ligado bool) error {
	var id int
	err := db.QueryRow(ctx,
		"SELECT id FROM service.wxs_settings ORDER BY id DESC LIMIT 1").Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoWxsSettings
	}
	if err != nil {
		return fmt.Errorf("erro ao localizar configuração de W-Access: %w", err)
	}

	_, err = db.Exec(ctx,
		"UPDATE service.wxs_settings SET sync_enabled = $1, updated_at = NOW() WHERE id = $2",
		ligado, id)
	if err != nil {
		return fmt.Errorf("erro ao gravar sync_enabled: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/database/ -v`
Expected: PASS

- [ ] **Step 5: Escrever o teste de órfãos por origem**

Criar `internal/emulator/sync_test.go`:

```go
package emulator

import (
	"reflect"
	"testing"

	"GoFacialEmulator/internal/models"
)

// A regra que sustenta a coexistência: sync só apaga o que o sync criou.
// Sem isso, o primeiro refresh depois de um cadastro manual apagaria o
// cadastro.
func TestOrphanIDsPreservaManuais(t *testing.T) {
	devices := []models.Device{
		{ID: 17, Source: SourceWXS},     // ainda existe no W-Access
		{ID: 18, Source: SourceWXS},     // sumiu do W-Access — órfão
		{ID: 900001, Source: SourceManual}, // cadastrado à mão
		{ID: 900002, Source: SourceManual},
	}
	validos := map[int]bool{17: true}

	got := orphanIDs(devices, validos)

	if quero := []int{18}; !reflect.DeepEqual(got, quero) {
		t.Errorf("órfãos: %v, quero %v", got, quero)
	}
}

func TestOrphanIDsSemOrfaos(t *testing.T) {
	devices := []models.Device{{ID: 17, Source: SourceWXS}}
	if got := orphanIDs(devices, map[int]bool{17: true}); len(got) != 0 {
		t.Errorf("quero nenhum órfão, tenho %v", got)
	}
}

// Dispositivo sem origem gravada é anterior à V002 — tratar como wxs, que
// é o que o DEFAULT da migração diz.
func TestOrphanIDsTrataSourceVazioComoWXS(t *testing.T) {
	devices := []models.Device{{ID: 18, Source: ""}}
	if got := orphanIDs(devices, map[int]bool{}); !reflect.DeepEqual(got, []int{18}) {
		t.Errorf("órfãos: %v, quero [18]", got)
	}
}
```

- [ ] **Step 6: Rodar e ver falhar**

Run: `go test ./internal/emulator/ -run TestOrphanIDs -v`
Expected: FAIL — `undefined: orphanIDs`

- [ ] **Step 7: Implementar `orphanIDs` e reescrever o cleanup**

Em `internal/emulator/manager.go`, substituir o corpo de `cleanupOrphanedDevices` (linha ~239) por:

```go
// orphanIDs devolve os dispositivos que o sync deve apagar: os de origem
// W-Access que não vieram na última resposta. Emulador manual nunca é
// órfão — ninguém além do próprio operador manda nele.
func orphanIDs(devices []models.Device, validos map[int]bool) []int {
	var orfaos []int
	for _, d := range devices {
		if d.Source == SourceManual {
			continue
		}
		if !validos[d.ID] {
			orfaos = append(orfaos, d.ID)
		}
	}
	return orfaos
}

// cleanupOrphanedDevices remove dispositivos que sumiram do W-Access.
func (m *Manager) cleanupOrphanedDevices(ctx context.Context, controllers []map[string]interface{}) error {
	validIDs := make(map[int]bool)
	for _, controller := range controllers {
		id := controller["LocalControllerID"].(int)
		validIDs[id] = true
	}

	devices, err := m.ListDevices()
	if err != nil {
		return err
	}

	for _, id := range orphanIDs(devices, validIDs) {
		if emulator, exists := m.emulators[id]; exists && emulator.IsRunning() {
			m.Stop(id)
		}

		// O AND source garante que uma corrida entre o refresh e um
		// cadastro manual não apague o cadastro: se a linha virou manual
		// entre o SELECT e o DELETE, o DELETE não pega nada.
		_, err := m.ServiceDB.Exec(ctx,
			"DELETE FROM service.devices WHERE local_controller_id = $1 AND source = 'wxs'", id)
		if err != nil {
			m.Tracer.Error("Failed to delete orphaned device %d: %v", id, err)
		}

		m.watchdogMutex.Lock()
		delete(m.watchdog, id)
		m.watchdogMutex.Unlock()
	}

	return nil
}
```

O `delete(m.watchdog, ...)` original estava sem mutex, enquanto o resto do arquivo protege esse mapa com `watchdogMutex` — corrigido junto porque a linha está sendo reescrita de qualquer forma.

- [ ] **Step 8: Escrever o teste do gate de sync**

Acrescentar em `internal/emulator/sync_test.go`:

```go
import (
	"errors"

	"GoFacialEmulator/internal/trace"
)

// Sem WxsDB não há o que sincronizar, e o erro precisa ser distinguível
// de "WXS fora do ar" — o handler devolve 409 num caso e 502 no outro.
func TestRefreshDevicesSemWxsDBDevolveSyncDisabled(t *testing.T) {
	m := &Manager{
		ServiceDB: &dbFalso{},
		Tracer:    trace.NewTracer(),
		emulators: map[int]Emulator{},
		watchdog:  map[int]*WatchdogInfo{},
	}

	err := m.RefreshDevices()
	if !errors.Is(err, ErrSyncDisabled) {
		t.Errorf("quero ErrSyncDisabled, tenho %v", err)
	}
}

// Um refresh que falhou no gate não pode deixar a flag presa em true, ou
// o próximo refresh legítimo seria recusado como "já em andamento".
func TestRefreshDevicesLiberaFlagAposGate(t *testing.T) {
	m := &Manager{
		ServiceDB: &dbFalso{},
		Tracer:    trace.NewTracer(),
		emulators: map[int]Emulator{},
		watchdog:  map[int]*WatchdogInfo{},
	}

	_ = m.RefreshDevices()

	if m.IsRefreshInProgress() {
		t.Error("flag de refresh ficou presa em true")
	}
}
```

- [ ] **Step 9: Rodar e ver falhar**

Run: `go test ./internal/emulator/ -run TestRefreshDevices -v`
Expected: FAIL — o erro atual é um `fmt.Errorf` sem `ErrSyncDisabled`

- [ ] **Step 10: Colocar o gate em `RefreshDevices`**

Em `internal/emulator/manager.go`, substituir o bloco de verificação de `m.WxsDB` (linha ~124) por:

```go
	// O vínculo com o Invenzi é opcional. Erro tipado, e não fmt.Errorf,
	// porque o handler precisa distinguir "sync desligado" (estado
	// esperado, 409) de "W-Access fora do ar" (falha, 502).
	if m.WxsDB == nil {
		return fmt.Errorf("%w: W-Access não configurado", ErrSyncDisabled)
	}

	gateCtx, gateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	ligado, err := database.GetSyncEnabled(gateCtx, m.ServiceDB)
	gateCancel()
	if err != nil {
		return fmt.Errorf("failed to read sync setting: %w", err)
	}
	if !ligado {
		return fmt.Errorf("%w: desligada nas configurações", ErrSyncDisabled)
	}
```

Atenção: a checagem fica **depois** do `CompareAndSwap` e do `defer m.refreshInProgress.Store(false)` que já existem no topo da função, para a flag ser liberada de qualquer jeito.

- [ ] **Step 11: Rodar e ver passar**

Run: `go test ./internal/emulator/ -v`
Expected: PASS

- [ ] **Step 12: Escrever o teste do handler do toggle**

Criar `internal/handlers/sync_settings_test.go`:

```go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestApiSetSyncEnabledRejeitaCorpoInvalido(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{serviceDB: &dbFalso{}, tracer: tracerDeTeste(t)}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/settings/sync",
		strings.NewReader(`{"enabled":"talvez"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.apiSetSyncEnabled(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, quero 400", rec.Code)
	}
}
```

Se ainda não existir um helper de tracer nos testes de `handlers`, acrescentar em `internal/handlers/sync_settings_test.go`:

```go
import "GoFacialEmulator/internal/trace"

func tracerDeTeste(t *testing.T) *trace.Tracer {
	t.Helper()
	return trace.NewTracer()
}
```

- [ ] **Step 13: Rodar e ver falhar**

Run: `go test ./internal/handlers/ -run TestApiSetSyncEnabled -v`
Expected: FAIL — `h.apiSetSyncEnabled undefined`

- [ ] **Step 14: Implementar o handler do toggle**

Criar `internal/handlers/sync_settings.go`:

```go
package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"GoFacialEmulator/internal/database"

	"github.com/gin-gonic/gin"
)

// apiSetSyncEnabled liga ou desliga o vínculo com o Invenzi W-Access.
// Desligado, RefreshDevices recusa e o serviço passa a viver só dos
// emuladores cadastrados na mão.
func (h *Handler) apiSetSyncEnabled(c *gin.Context) {
	var corpo struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&corpo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := database.SetSyncEnabled(ctx, h.serviceDB, corpo.Enabled); err != nil {
		if errors.Is(err, database.ErrNoWxsSettings) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Configure a conexão com o W-Access antes de ligar a sincronização",
			})
			return
		}
		h.tracer.Error("Failed to set sync_enabled: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao gravar configuração"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"enabled": corpo.Enabled})
}
```

- [ ] **Step 15: Registrar a rota e tratar o gate nos dois refreshes**

Em `internal/handlers/handlers.go`, no grupo `settings` de `setupAPIRoutes` (linha ~245):

```go
			settings.POST("/sync", h.apiSetSyncEnabled)
```

Substituir `refreshDevices` (linha ~406) por:

```go
func (h *Handler) refreshDevices(c *gin.Context) {
	h.tracer.Info(">>> Refreshing database")

	// O gate é lido antes de disparar o trabalho: sync desligado é um
	// estado que o operador precisa ver como recusa, não como um refresh
	// que "iniciou" e morreu calado numa goroutine.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	ligado, err := database.GetSyncEnabled(ctx, h.serviceDB)
	cancel()
	if err != nil {
		h.tracer.Error("Failed to read sync setting: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erro ao ler configuração de sincronização"})
		return
	}
	if !ligado || h.manager.WxsDB == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Sincronização com o W-Access está desligada",
		})
		return
	}

	go func() {
		if err := h.manager.RefreshDevices(); err != nil {
			h.tracer.Error("Failed to refresh devices: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "Refresh iniciado em background",
		"status":  "started",
	})
}
```

Em `apiRefreshEmulators` (linha ~513), que chama `RefreshDevices` de forma síncrona, mapear o erro:

```go
	if err := h.manager.RefreshDevices(); err != nil {
		if errors.Is(err, emulator.ErrSyncDisabled) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
```

Acrescentar `"errors"` aos imports de `handlers.go` se ainda não estiver lá.

- [ ] **Step 16: Passar o estado do toggle para a tela de configurações**

Em `settingsPage` (linha ~906), o `GetWxsSettingsFromDB` devolve erro quando não há linha — hoje isso derruba a página inteira em 500 numa instalação nova. Substituir por:

```go
func (h *Handler) settingsPage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Sem linha gravada a tela precisa abrir mesmo assim, com os campos
	// vazios — é exatamente a tela que o cliente usa para gravar a
	// primeira configuração.
	wxsSettings, err := database.GetWxsSettingsFromDB(ctx, h.serviceDB)
	if err != nil {
		h.tracer.Info("No WXS settings yet: %v", err)
		wxsSettings = &database.WxsSettings{}
	}

	syncLigado, err := database.GetSyncEnabled(ctx, h.serviceDB)
	if err != nil {
		h.tracer.Error("Failed to read sync setting: %v", err)
		syncLigado = false
	}

	h.renderPage(c, "settings.html", http.StatusOK, gin.H{
		"wxs_settings": wxsSettings,
		"sync_enabled": syncLigado,
	})
}
```

- [ ] **Step 17: Rodar a suíte inteira**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 18: Commit**

```bash
git add internal/database/sync_settings.go internal/database/sync_settings_test.go internal/emulator/manager.go internal/emulator/sync_test.go internal/handlers/sync_settings.go internal/handlers/sync_settings_test.go internal/handlers/handlers.go
git commit -m "feat(sync): vínculo com o W-Access vira opção e cleanup respeita a origem"
```

---

### Task 5: CRUD no manager

**Files:**
- Create: `internal/emulator/crud.go`
- Create: `internal/emulator/crud_test.go`
- Modify: `internal/emulator/manager.go:19-51` (campo novo no struct)
- Modify: `cmd/emulator-service/main.go` (informar a porta do serviço ao manager)

**Interfaces:**
- Consumes: `DeviceSpec`, `RangeSpec`, `ConflictError`, `Conflicts`, `ErrDeviceIsManaged`, `ErrDeviceRunning`, `ErrDeviceNotFound`, `SourceManual` (Task 3); `models.Device`
- Produces: `Manager.ServicePort int`; `(*Manager).CreateDevice(ctx, DeviceSpec) (models.Device, error)`; `(*Manager).CreateDeviceRange(ctx, RangeSpec) ([]models.Device, error)`; `(*Manager).UpdateDevice(ctx, int, DeviceSpec) (models.Device, error)`; `(*Manager).DeleteDevice(ctx, int) error`

- [ ] **Step 1: Escrever os testes**

Criar `internal/emulator/crud_test.go`:

```go
package emulator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"

	"github.com/jackc/pgx/v4"
)

// managerDeTeste devolve um Manager apoiado em fakes, com a porta do
// serviço em 8080 (o padrão do config.yaml).
func managerDeTeste(tx *txFalsa) (*Manager, *dbFalso) {
	db := &dbFalso{tx: tx}
	return &Manager{
		ServiceDB:   db,
		EmulatorDB:  db,
		Tracer:      trace.NewTracer(),
		ServicePort: 8080,
		emulators:   map[int]Emulator{},
		watchdog:    map[int]*WatchdogInfo{},
		startErrors: map[int]string{},
	}, db
}

// idSequencial simula nextval devolvendo 900001, 900002, ...
func idSequencial(inicio int) func(sql string) pgx.Row {
	proximo := inicio
	return func(sql string) pgx.Row {
		if strings.Contains(sql, "nextval") {
			id := proximo
			proximo++
			return linhaFalsa{valores: []interface{}{id}}
		}
		return linhaFalsa{}
	}
}

func TestCreateDeviceGravaComoManual(t *testing.T) {
	tx := &txFalsa{queryRowFn: idSequencial(900001)}
	m, _ := managerDeTeste(tx)

	dev, err := m.CreateDevice(context.Background(), DeviceSpec{
		Name: "lab-01", Model: ModelDahua, Port: 4000,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if dev.ID != 900001 {
		t.Errorf("id %d, quero 900001", dev.ID)
	}
	if dev.Source != SourceManual {
		t.Errorf("source %q, quero %q", dev.Source, SourceManual)
	}
	if dev.Status != "stopped" {
		t.Errorf("status %q, quero stopped", dev.Status)
	}
	if dev.IPAddress != IPPadrao {
		t.Errorf("ip %q, quero o padrão %q", dev.IPAddress, IPPadrao)
	}
	if !tx.comitou {
		t.Error("quero commit")
	}

	juntos := strings.Join(tx.execs, "\n")
	if !strings.Contains(juntos, "pg_advisory_xact_lock") {
		t.Error("quero o lock que serializa duas criações concorrentes")
	}
	if !strings.Contains(juntos, "INSERT INTO service.devices") {
		t.Error("quero o INSERT do dispositivo")
	}
}

func TestCreateDeviceRejeitaSpecInvalida(t *testing.T) {
	tx := &txFalsa{queryRowFn: idSequencial(900001)}
	m, _ := managerDeTeste(tx)

	_, err := m.CreateDevice(context.Background(), DeviceSpec{Name: "x", Model: "Nedap", Port: 4000})

	if !errors.Is(err, ErrInvalidSpec) {
		t.Errorf("quero ErrInvalidSpec, tenho %v", err)
	}
	if len(tx.execs) != 0 {
		t.Errorf("nada podia ter sido executado, tenho %v", tx.execs)
	}
}

func TestCreateDeviceRecusaPortaOcupada(t *testing.T) {
	tx := &txFalsa{
		queryRowFn: idSequencial(900001),
		queryRows:  &rowsFalsas{linhas: [][]interface{}{{4000}, {7070}}},
	}
	m, _ := managerDeTeste(tx)

	_, err := m.CreateDevice(context.Background(), DeviceSpec{
		Name: "lab-01", Model: ModelDahua, Port: 4000,
	})

	var conf *ConflictError
	if !errors.As(err, &conf) {
		t.Fatalf("quero ConflictError, tenho %v", err)
	}
	if len(conf.Ports) != 1 || conf.Ports[0] != 4000 {
		t.Errorf("conflitos: %v, quero [4000]", conf.Ports)
	}
	if tx.comitou {
		t.Error("transação não podia ter sido confirmada")
	}
}

func TestCreateDeviceRecusaPortaDoProprioServico(t *testing.T) {
	tx := &txFalsa{queryRowFn: idSequencial(900001)}
	m, _ := managerDeTeste(tx) // ServicePort = 8080

	_, err := m.CreateDevice(context.Background(), DeviceSpec{
		Name: "colide", Model: ModelDahua, Port: 8080,
	})

	var conf *ConflictError
	if !errors.As(err, &conf) {
		t.Fatalf("quero ConflictError, tenho %v", err)
	}
	if len(conf.Reserved) != 1 || conf.Reserved[0] != 8080 {
		t.Errorf("reservadas: %v, quero [8080]", conf.Reserved)
	}
}

func TestCreateDeviceRangeCriaTodasAsPortas(t *testing.T) {
	tx := &txFalsa{queryRowFn: idSequencial(900001)}
	m, _ := managerDeTeste(tx)

	devs, err := m.CreateDeviceRange(context.Background(), RangeSpec{
		NamePrefix: "lab", Model: ModelHikvision, PortStart: 4000, PortEnd: 4002,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(devs) != 3 {
		t.Fatalf("quero 3 dispositivos, tenho %d", len(devs))
	}
	if devs[0].Name != "lab-4000" || devs[2].Name != "lab-4002" {
		t.Errorf("nomes gerados: %q ... %q", devs[0].Name, devs[2].Name)
	}
	if devs[0].ID == devs[1].ID {
		t.Error("IDs repetidos no lote")
	}
	if !tx.comitou {
		t.Error("quero commit")
	}
}

// Colisão em qualquer porta do lote não grava nada — é o comportamento
// atômico que a spec exige.
func TestCreateDeviceRangeComColisaoNaoGravaNada(t *testing.T) {
	tx := &txFalsa{
		queryRowFn: idSequencial(900001),
		queryRows:  &rowsFalsas{linhas: [][]interface{}{{4001}}},
	}
	m, _ := managerDeTeste(tx)

	_, err := m.CreateDeviceRange(context.Background(), RangeSpec{
		NamePrefix: "lab", Model: ModelDahua, PortStart: 4000, PortEnd: 4002,
	})

	var conf *ConflictError
	if !errors.As(err, &conf) {
		t.Fatalf("quero ConflictError, tenho %v", err)
	}
	for _, sql := range tx.execs {
		if strings.Contains(sql, "INSERT INTO service.devices") {
			t.Error("nenhum INSERT podia ter acontecido")
		}
	}
	if tx.comitou {
		t.Error("transação não podia ter sido confirmada")
	}
}

func TestDeleteDevicePurgaTodasAsTabelas(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}

	if err := m.DeleteDevice(context.Background(), 900001); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	juntos := strings.Join(tx.execs, "\n")
	tabelas := []string{
		"emulator.dahua_cards", "emulator.dahua_faces",
		"emulator.hikvision_users", "emulator.hikvision_cards",
		"emulator.hikvision_faces", "emulator.hikvision_fingers",
		"emulator.device_settings", "service.users_comparison",
		"service.devices",
	}
	for _, tab := range tabelas {
		if !strings.Contains(juntos, tab) {
			t.Errorf("faltou limpar %s", tab)
		}
	}
	if !tx.comitou {
		t.Error("quero commit")
	}
}

// A linha device_id = 0 de emulator.device_settings guarda os padrões
// globais semeados na V001. Purgar por device_id específico nunca pode
// alcançá-la.
func TestDeleteDeviceNaoTocaNosPadroesGlobais(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}

	if err := m.DeleteDevice(context.Background(), 900001); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	for i, sql := range tx.execs {
		if strings.Contains(sql, "device_settings") {
			args := tx.execArgs[i]
			if len(args) == 0 || args[0].(int) != 900001 {
				t.Errorf("purga de device_settings com args %v", args)
			}
		}
	}
}

func TestDeleteDeviceRecusaDispositivoDoWXS(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceWXS}}

	err := m.DeleteDevice(context.Background(), 17)

	if !errors.Is(err, ErrDeviceIsManaged) {
		t.Errorf("quero ErrDeviceIsManaged, tenho %v", err)
	}
}

func TestUpdateDeviceRecusaDispositivoDoWXS(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceWXS}}

	_, err := m.UpdateDevice(context.Background(), 17, DeviceSpec{
		Name: "novo", Model: ModelDahua, Port: 4000,
	})

	if !errors.Is(err, ErrDeviceIsManaged) {
		t.Errorf("quero ErrDeviceIsManaged, tenho %v", err)
	}
}

// emuladorFalso satisfaz a interface Emulator para simular um em execução.
type emuladorFalso struct{ rodando bool }

func (e *emuladorFalso) Start() error                 { return nil }
func (e *emuladorFalso) Stop() error                  { e.rodando = false; return nil }
func (e *emuladorFalso) IsRunning() bool              { return e.rodando }
func (e *emuladorFalso) GetInfo() models.Device       { return models.Device{} }
func (e *emuladorFalso) GenerateEvent() error         { return nil }
func (e *emuladorFalso) GetType() string              { return ModelDahua }
func (e *emuladorFalso) GetTotalUsers() (int, error)  { return 0, nil }

func TestUpdateDeviceRecusaEmuladorRodando(t *testing.T) {
	tx := &txFalsa{}
	m, db := managerDeTeste(tx)
	db.linha = linhaFalsa{valores: []interface{}{SourceManual}}
	m.emulators[900001] = &emuladorFalso{rodando: true}

	_, err := m.UpdateDevice(context.Background(), 900001, DeviceSpec{
		Name: "novo", Model: ModelDahua, Port: 4000,
	})

	if !errors.Is(err, ErrDeviceRunning) {
		t.Errorf("quero ErrDeviceRunning, tenho %v", err)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/emulator/ -run 'TestCreateDevice|TestDeleteDevice|TestUpdateDevice' -v`
Expected: FAIL — `m.CreateDevice undefined`

- [ ] **Step 3: Acrescentar `ServicePort` ao Manager**

Em `internal/emulator/manager.go`, no struct `Manager`, depois de `Tracer`:

```go
	// ServicePort é a porta HTTP do próprio serviço. Cadastrar um emulador
	// nela produziria um dispositivo que nunca consegue subir, e o erro
	// apareceria só no start, longe da causa.
	ServicePort int
```

Em `cmd/emulator-service/main.go`, logo depois de `manager := emulator.NewManager(...)`:

```go
	manager.ServicePort = cfg.Server.Port
```

- [ ] **Step 4: Implementar `crud.go`**

Criar `internal/emulator/crud.go`:

```go
package emulator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"GoFacialEmulator/internal/models"

	"github.com/jackc/pgx/v4"
)

// chaveLockCriacao serializa a criação de emuladores. A validação de porta
// e a gravação precisam ser atômicas entre si: sem o lock, duas criações
// simultâneas passam as duas pela validação e colidem na gravação.
//
// Um UNIQUE em service.devices(port) seria mais direto, mas quebraria a
// migração de qualquer instalação cujo W-Access já tenha portas repetidas,
// e o esquema atual não impede isso.
const chaveLockCriacao int64 = 8080_4000

// CreateDevice cadastra um emulador manual.
func (m *Manager) CreateDevice(ctx context.Context, spec DeviceSpec) (models.Device, error) {
	if err := spec.Normalize(); err != nil {
		return models.Device{}, err
	}

	devices, err := m.criarVarios(ctx, []DeviceSpec{spec})
	if err != nil {
		return models.Device{}, err
	}
	return devices[0], nil
}

// CreateDeviceRange cadastra um emulador por porta do intervalo. Ou todos
// entram, ou nenhum entra.
func (m *Manager) CreateDeviceRange(ctx context.Context, spec RangeSpec) ([]models.Device, error) {
	if err := spec.Normalize(); err != nil {
		return nil, err
	}
	return m.criarVarios(ctx, spec.Expand())
}

// criarVarios é o caminho único de gravação dos dois verbos de criação.
func (m *Manager) criarVarios(ctx context.Context, specs []DeviceSpec) ([]models.Device, error) {
	tx, err := m.ServiceDB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chaveLockCriacao); err != nil {
		return nil, fmt.Errorf("erro ao obter lock de criação: %w", err)
	}

	ocupadas, err := portasOcupadas(ctx, tx)
	if err != nil {
		return nil, err
	}

	desejadas := make([]int, 0, len(specs))
	for _, s := range specs {
		desejadas = append(desejadas, s.Port)
	}

	conflito := &ConflictError{Ports: Conflicts(desejadas, ocupadas)}
	if m.ServicePort > 0 {
		conflito.Reserved = Conflicts(desejadas, map[int]bool{m.ServicePort: true})
	}
	if len(conflito.Ports) > 0 || len(conflito.Reserved) > 0 {
		return nil, conflito
	}

	devices := make([]models.Device, 0, len(specs))
	for _, s := range specs {
		var id int
		if err := tx.QueryRow(ctx,
			"SELECT nextval('service.manual_device_id_seq')").Scan(&id); err != nil {
			return nil, fmt.Errorf("erro ao alocar id: %w", err)
		}

		dev := models.Device{
			ID:            id,
			Name:          s.Name,
			IPAddress:     s.IPAddress,
			Port:          s.Port,
			Model:         s.Model,
			Enabled:       boolParaInt(*s.Enabled),
			Type:          m.getDeviceType(s.Model),
			Status:        "stopped",
			EventInterval: s.EventInterval,
			TotalUsers:    0,
			LogEnabled:    0,
			Source:        SourceManual,
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO service.devices (
				local_controller_id, name, ip_address, port, model, enabled, type,
				status, event_interval, total_users, log_enabled, source
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'manual')`,
			dev.ID, dev.Name, dev.IPAddress, dev.Port, dev.Model, dev.Enabled,
			dev.Type, dev.Status, dev.EventInterval, dev.TotalUsers, dev.LogEnabled)
		if err != nil {
			return nil, fmt.Errorf("erro ao inserir dispositivo na porta %d: %w", s.Port, err)
		}

		devices = append(devices, dev)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("erro ao confirmar criação: %w", err)
	}

	// Só depois do commit: watchdog apontando para dispositivo que não
	// existe geraria health check em cima de nada.
	m.watchdogMutex.Lock()
	for _, dev := range devices {
		m.watchdog[dev.ID] = &WatchdogInfo{LastCheck: time.Now(), LastStatus: "stopped"}
	}
	m.watchdogMutex.Unlock()

	return devices, nil
}

// portasOcupadas lê todas as portas já cadastradas, de qualquer origem —
// um emulador manual não pode colidir nem com outro manual nem com um
// dispositivo vindo do W-Access.
func portasOcupadas(ctx context.Context, tx pgx.Tx) (map[int]bool, error) {
	rows, err := tx.Query(ctx, "SELECT port FROM service.devices")
	if err != nil {
		return nil, fmt.Errorf("erro ao ler portas cadastradas: %w", err)
	}
	defer rows.Close()

	ocupadas := map[int]bool{}
	for rows.Next() {
		var porta int
		if err := rows.Scan(&porta); err != nil {
			return nil, fmt.Errorf("erro ao ler porta cadastrada: %w", err)
		}
		ocupadas[porta] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar portas cadastradas: %w", err)
	}
	return ocupadas, nil
}

// UpdateDevice substitui os campos editáveis de um emulador manual parado.
func (m *Manager) UpdateDevice(ctx context.Context, id int, spec DeviceSpec) (models.Device, error) {
	if err := spec.Normalize(); err != nil {
		return models.Device{}, err
	}

	if err := m.exigeManualEParado(ctx, id); err != nil {
		return models.Device{}, err
	}

	tx, err := m.ServiceDB.Begin(ctx)
	if err != nil {
		return models.Device{}, fmt.Errorf("erro ao abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chaveLockCriacao); err != nil {
		return models.Device{}, fmt.Errorf("erro ao obter lock: %w", err)
	}

	ocupadas, err := portasOcupadas(ctx, tx)
	if err != nil {
		return models.Device{}, err
	}
	// A porta atual do próprio dispositivo não conta como conflito: manter
	// a porta é o caso mais comum de edição.
	atual, err := m.GetDevice(id)
	if err != nil {
		return models.Device{}, err
	}
	delete(ocupadas, atual.Port)

	conflito := &ConflictError{Ports: Conflicts([]int{spec.Port}, ocupadas)}
	if m.ServicePort > 0 {
		conflito.Reserved = Conflicts([]int{spec.Port}, map[int]bool{m.ServicePort: true})
	}
	if len(conflito.Ports) > 0 || len(conflito.Reserved) > 0 {
		return models.Device{}, conflito
	}

	_, err = tx.Exec(ctx, `
		UPDATE service.devices
		   SET name = $2, ip_address = $3, port = $4, model = $5,
		       enabled = $6, type = $7, event_interval = $8, updated_at = NOW()
		 WHERE local_controller_id = $1 AND source = 'manual'`,
		id, spec.Name, spec.IPAddress, spec.Port, spec.Model,
		boolParaInt(*spec.Enabled), m.getDeviceType(spec.Model), spec.EventInterval)
	if err != nil {
		return models.Device{}, fmt.Errorf("erro ao atualizar dispositivo %d: %w", id, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Device{}, fmt.Errorf("erro ao confirmar atualização: %w", err)
	}

	return models.Device{
		ID: id, Name: spec.Name, IPAddress: spec.IPAddress, Port: spec.Port,
		Model: spec.Model, Enabled: boolParaInt(*spec.Enabled),
		Type: m.getDeviceType(spec.Model), Status: "stopped",
		EventInterval: spec.EventInterval, Source: SourceManual,
	}, nil
}

// tabelasDeDados são todas as tabelas com device_id solto. Não há FK no
// esquema, então a limpeza é explícita.
var tabelasDeDados = []string{
	"emulator.dahua_cards",
	"emulator.dahua_faces",
	"emulator.hikvision_users",
	"emulator.hikvision_cards",
	"emulator.hikvision_faces",
	"emulator.hikvision_fingers",
	"emulator.device_settings",
}

// DeleteDevice remove um emulador manual e tudo que ele acumulou. Para o
// emulador antes, sem exigir que o operador pare na mão: remover é uma
// intenção inequívoca, editar não.
func (m *Manager) DeleteDevice(ctx context.Context, id int) error {
	origem, err := m.deviceSource(ctx, id)
	if err != nil {
		return err
	}
	if origem != SourceManual {
		return fmt.Errorf("%w: dispositivo %d", ErrDeviceIsManaged, id)
	}

	m.emulatorMutex.RLock()
	inst, rodando := m.emulators[id]
	m.emulatorMutex.RUnlock()
	if rodando && inst.IsRunning() {
		if err := m.Stop(id); err != nil {
			return fmt.Errorf("erro ao parar emulador %d antes de remover: %w", id, err)
		}
	}

	tx, err := m.ServiceDB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erro ao abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, tabela := range tabelasDeDados {
		// Filtro por device_id específico: a linha device_id = 0 de
		// device_settings guarda os padrões globais e não pertence a
		// dispositivo nenhum.
		if _, err := tx.Exec(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE device_id = $1", tabela), id); err != nil {
			return fmt.Errorf("erro ao limpar %s do dispositivo %d: %w", tabela, id, err)
		}
	}

	if _, err := tx.Exec(ctx,
		"DELETE FROM service.users_comparison WHERE local_controller_id = $1", id); err != nil {
		return fmt.Errorf("erro ao limpar comparação do dispositivo %d: %w", id, err)
	}

	if _, err := tx.Exec(ctx,
		"DELETE FROM service.devices WHERE local_controller_id = $1 AND source = 'manual'", id); err != nil {
		return fmt.Errorf("erro ao remover dispositivo %d: %w", id, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("erro ao confirmar remoção: %w", err)
	}

	m.watchdogMutex.Lock()
	delete(m.watchdog, id)
	m.watchdogMutex.Unlock()

	m.emulatorMutex.Lock()
	delete(m.emulators, id)
	m.emulatorMutex.Unlock()

	m.Tracer.Info("Removed manual device %d", id)
	return nil
}

// deviceSource lê a origem de um dispositivo.
func (m *Manager) deviceSource(ctx context.Context, id int) (string, error) {
	var origem string
	err := m.ServiceDB.QueryRow(ctx,
		"SELECT source FROM service.devices WHERE local_controller_id = $1", id).Scan(&origem)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: %d", ErrDeviceNotFound, id)
	}
	if err != nil {
		return "", fmt.Errorf("erro ao ler origem do dispositivo %d: %w", id, err)
	}
	if origem == "" {
		// Linha anterior à V002: o DEFAULT da migração diz wxs.
		origem = SourceWXS
	}
	return origem, nil
}

// exigeManualEParado é a guarda comum da edição.
func (m *Manager) exigeManualEParado(ctx context.Context, id int) error {
	origem, err := m.deviceSource(ctx, id)
	if err != nil {
		return err
	}
	if origem != SourceManual {
		return fmt.Errorf("%w: dispositivo %d", ErrDeviceIsManaged, id)
	}

	m.emulatorMutex.RLock()
	inst, existe := m.emulators[id]
	m.emulatorMutex.RUnlock()
	if existe && inst.IsRunning() {
		return fmt.Errorf("%w: pare o emulador %d antes de editar", ErrDeviceRunning, id)
	}
	return nil
}

func boolParaInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 5: Rodar e ver passar**

Run: `go test ./internal/emulator/ -v`
Expected: PASS

- [ ] **Step 6: Compilar tudo**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/emulator/crud.go internal/emulator/crud_test.go internal/emulator/manager.go cmd/emulator-service/main.go
git commit -m "feat(emulator): CRUD de emuladores manuais com criação em lote e purga"
```

---

### Task 6: Rotas `/api/emulators`

**Files:**
- Create: `internal/handlers/emulators.go`
- Create: `internal/handlers/emulators_http_test.go`
- Modify: `internal/handlers/handlers.go:213-253` (`setupAPIRoutes`)

**Interfaces:**
- Consumes: `(*Manager).CreateDevice/CreateDeviceRange/UpdateDevice/DeleteDevice`, `ConflictError`, `ErrInvalidSpec`, `ErrDeviceIsManaged`, `ErrDeviceRunning`, `ErrDeviceNotFound`
- Produces: rotas `GET/POST /api/emulators`, `POST /api/emulators/range`, `PUT/DELETE /api/emulators/:id`; handlers `apiListEmulators`, `apiCreateEmulator`, `apiCreateEmulatorRange`, `apiUpdateEmulator`, `apiDeleteEmulator`

- [ ] **Step 1: Escrever os testes de contrato HTTP**

Criar `internal/handlers/emulators_http_test.go`:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"GoFacialEmulator/internal/emulator"

	"github.com/gin-gonic/gin"
)

// requisicao monta um contexto Gin com corpo JSON e parâmetros de rota.
func requisicao(t *testing.T, metodo, alvo, corpo string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	return c, rec
}

func TestApiCreateEmulatorRejeitaJSONQuebrado(t *testing.T) {
	h := &Handler{tracer: tracerDeTeste(t)}

	c, rec := requisicao(t, http.MethodPost, "/api/emulators", `{"name":`, nil)
	h.apiCreateEmulator(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, quero 400", rec.Code)
	}
}

func TestApiUpdateEmulatorRejeitaIDNaoNumerico(t *testing.T) {
	h := &Handler{tracer: tracerDeTeste(t)}

	c, rec := requisicao(t, http.MethodPut, "/api/emulators/abc", `{}`,
		gin.Params{{Key: "id", Value: "abc"}})
	h.apiUpdateEmulator(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, quero 400", rec.Code)
	}
}

func TestApiDeleteEmulatorRejeitaIDNaoNumerico(t *testing.T) {
	h := &Handler{tracer: tracerDeTeste(t)}

	c, rec := requisicao(t, http.MethodDelete, "/api/emulators/xyz", "",
		gin.Params{{Key: "id", Value: "xyz"}})
	h.apiDeleteEmulator(c)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, quero 400", rec.Code)
	}
}

// A tradução de erro de domínio em status HTTP é o contrato que a UI e
// qualquer integração consomem — vale testar cada ramo.
func TestStatusDoErroDeDominio(t *testing.T) {
	casos := []struct {
		nome   string
		err    error
		quero  int
	}{
		{"spec inválida", emulator.ErrInvalidSpec, http.StatusBadRequest},
		{"conflito de porta", &emulator.ConflictError{Ports: []int{4000}}, http.StatusBadRequest},
		{"não encontrado", emulator.ErrDeviceNotFound, http.StatusNotFound},
		{"gerenciado pelo W-Access", emulator.ErrDeviceIsManaged, http.StatusConflict},
		{"emulador rodando", emulator.ErrDeviceRunning, http.StatusConflict},
		{"falha de banco", errBancoDeTeste, http.StatusInternalServerError},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := statusDoErro(c.err); got != c.quero {
				t.Errorf("status %d, quero %d", got, c.quero)
			}
		})
	}
}

// O corpo do erro de conflito precisa listar as portas: a UI monta a
// mensagem em cima dessa lista.
func TestCorpoDoErroDeConflitoListaPortas(t *testing.T) {
	h := &Handler{tracer: tracerDeTeste(t)}

	c, rec := requisicao(t, http.MethodPost, "/api/emulators", "", nil)
	h.responderErro(c, &emulator.ConflictError{Ports: []int{4001, 4003}})

	var corpo struct {
		Error     string `json:"error"`
		Conflicts []int  `json:"conflicts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("corpo não é JSON: %v — %s", err, rec.Body.String())
	}
	if len(corpo.Conflicts) != 2 || corpo.Conflicts[0] != 4001 {
		t.Errorf("conflicts: %v, quero [4001 4003]", corpo.Conflicts)
	}
	if corpo.Error == "" {
		t.Error("quero uma mensagem legível em error")
	}
}
```

Acrescentar ao mesmo arquivo, junto dos imports:

```go
import "errors"

// errBancoDeTeste representa qualquer falha de infraestrutura.
var errBancoDeTeste = errors.New("connection refused")
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/handlers/ -run 'TestApiCreate|TestApiUpdate|TestApiDelete|TestStatusDoErro|TestCorpoDoErro' -v`
Expected: FAIL — `h.apiCreateEmulator undefined`

- [ ] **Step 3: Implementar `emulators.go`**

Criar `internal/handlers/emulators.go`:

```go
package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"GoFacialEmulator/internal/emulator"

	"github.com/gin-gonic/gin"
)

// statusDoErro traduz erro de domínio em status HTTP. Concentrado numa
// função só para os cinco handlers responderem igual.
func statusDoErro(err error) int {
	switch {
	case errors.Is(err, emulator.ErrDeviceNotFound):
		return http.StatusNotFound
	case errors.Is(err, emulator.ErrDeviceIsManaged), errors.Is(err, emulator.ErrDeviceRunning):
		return http.StatusConflict
	case errors.Is(err, emulator.ErrInvalidSpec):
		// ConflictError também cai aqui: é erro de payload, não de estado.
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// responderErro escreve o corpo de erro. Conflito de porta ganha a lista
// crua além da mensagem, porque a UI destaca as portas problemáticas.
func (h *Handler) responderErro(c *gin.Context, err error) {
	status := statusDoErro(err)

	var conf *emulator.ConflictError
	if errors.As(err, &conf) {
		corpo := gin.H{"error": conf.Error()}
		if len(conf.Ports) > 0 {
			corpo["conflicts"] = conf.Ports
		}
		if len(conf.Reserved) > 0 {
			corpo["reserved"] = conf.Reserved
		}
		c.JSON(status, corpo)
		return
	}

	if status == http.StatusInternalServerError {
		h.tracer.Error("Emulator CRUD failed: %v", err)
		c.JSON(status, gin.H{"error": "Erro interno ao processar a operação"})
		return
	}

	c.JSON(status, gin.H{"error": err.Error()})
}

// apiListEmulators lista todos os dispositivos, com a origem de cada um.
func (h *Handler) apiListEmulators(c *gin.Context) {
	devices, err := h.manager.ListDevices()
	if err != nil {
		h.responderErro(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"emulators": devices, "count": len(devices)})
}

// apiCreateEmulator cadastra um emulador manual.
func (h *Handler) apiCreateEmulator(c *gin.Context) {
	var spec emulator.DeviceSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dev, err := h.manager.CreateDevice(ctx, spec)
	if err != nil {
		h.responderErro(c, err)
		return
	}

	resposta := gin.H{"emulator": dev}
	if spec.AutoStart {
		if err := h.manager.Start(dev.ID); err != nil {
			// O cadastro deu certo; só o start falhou. 201 com o aviso
			// dentro é mais honesto que 500 sobre um recurso criado.
			resposta["start_error"] = err.Error()
		} else {
			resposta["started"] = true
		}
	}

	c.JSON(http.StatusCreated, resposta)
}

// apiCreateEmulatorRange cadastra um emulador por porta do intervalo.
func (h *Handler) apiCreateEmulatorRange(c *gin.Context) {
	var spec emulator.RangeSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	// Lote grande com auto_start sobe centenas de servidores HTTP; o
	// timeout precisa acomodar isso.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	devices, err := h.manager.CreateDeviceRange(ctx, spec)
	if err != nil {
		h.responderErro(c, err)
		return
	}

	criados := make([]gin.H, 0, len(devices))
	for _, d := range devices {
		criados = append(criados, gin.H{"id": d.ID, "name": d.Name, "port": d.Port})
	}

	resposta := gin.H{"count": len(devices), "created": criados}

	if spec.AutoStart {
		iniciados := 0
		var falhas []gin.H
		for _, d := range devices {
			if err := h.manager.Start(d.ID); err != nil {
				falhas = append(falhas, gin.H{"id": d.ID, "error": err.Error()})
				continue
			}
			iniciados++
		}
		resposta["started"] = iniciados
		if len(falhas) > 0 {
			resposta["start_errors"] = falhas
		}
	}

	c.JSON(http.StatusCreated, resposta)
}

// apiUpdateEmulator substitui os campos editáveis de um emulador manual.
func (h *Handler) apiUpdateEmulator(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	var spec emulator.DeviceSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dev, err := h.manager.UpdateDevice(ctx, id, spec)
	if err != nil {
		h.responderErro(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"emulator": dev})
}

// apiDeleteEmulator remove um emulador manual e purga os dados dele.
func (h *Handler) apiDeleteEmulator(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := h.manager.DeleteDevice(ctx, id); err != nil {
		h.responderErro(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"removed": id})
}
```

- [ ] **Step 4: Registrar as rotas**

O grupo `/api/emulators` **já existe** em `setupAPIRoutes` (linha ~218) com as rotas `GET /start`, `GET /stop` e `GET /refresh`. Não criar um segundo grupo com o mesmo prefixo: o Gin entra em pânico com rota duplicada, e `GET /api/emulators/start` não convive com `PUT /api/emulators/:id` — o roteador do Gin recusa um parâmetro coringa que conflita com segmentos estáticos irmãos.

Substituir o grupo inteiro por:

```go
		emulators := api.Group("/emulators")
		{
			emulators.GET("", h.apiListEmulators)
			emulators.POST("", h.apiCreateEmulator)
			emulators.POST("/range", h.apiCreateEmulatorRange)
			emulators.PUT("/:id", h.apiUpdateEmulator)
			emulators.DELETE("/:id", h.apiDeleteEmulator)

			// Controle da frota inteira. Movidas para /control porque
			// /api/emulators/start colidiria com /api/emulators/:id.
			control := emulators.Group("/control")
			{
				control.GET("/start", h.apiStartEmulators)
				control.GET("/stop", h.apiStopEmulators)
				control.GET("/refresh", h.apiRefreshEmulators)
			}
		}
```

- [ ] **Step 5: Encontrar e ajustar quem chamava as rotas antigas**

Run: `grep -rn "api/emulators/\(start\|stop\|refresh\)" --include=*.js --include=*.html --include=*.md --include=*.go .`

Trocar cada ocorrência por `/api/emulators/control/{start,stop,refresh}`.

- [ ] **Step 6: Rodar os testes**

Run: `go test ./internal/handlers/ -v`
Expected: PASS

- [ ] **Step 7: Verificar as rotas de ponta a ponta**

Run: `go build -o /tmp/emu ./cmd/emulator-service && go vet ./...`
Expected: build limpo. Se houver PostgreSQL disponível, subir o serviço e conferir:

```bash
curl -s -X POST localhost:8080/api/emulators \
  -H 'Content-Type: application/json' \
  -d '{"name":"lab-01","model":"Hikvision","port":4001}'

curl -s -X POST localhost:8080/api/emulators/range \
  -H 'Content-Type: application/json' \
  -d '{"name_prefix":"lab","model":"Dahua","port_start":4100,"port_end":4104}'

curl -s localhost:8080/api/emulators | head -c 400
curl -s -X DELETE localhost:8080/api/emulators/900001
```

Sem banco disponível, registrar no commit que a verificação manual ficou pendente.

- [ ] **Step 8: Commit**

```bash
git add internal/handlers/emulators.go internal/handlers/emulators_http_test.go internal/handlers/handlers.go
git commit -m "feat(api): CRUD de emuladores em /api/emulators"
```

---

### Task 7: Tela de dispositivos — origem, editar e remover

**Files:**
- Modify: `assets/web/templates/devices.html:66-165`
- Modify: `assets/web/static/js/devices.js`
- Modify: `assets/web/static/css/components.css`
- Modify: `internal/handlers/render_pages_test.go`

**Interfaces:**
- Consumes: chave `source` no mapa de dispositivo (Task 2); `PUT`/`DELETE /api/emulators/:id` (Task 6); `showToast` de `assets/web/static/js/toast.js`
- Produces: coluna "Origem" na grade; botões `.device-edit` e `.device-remove` com `data-id`; modal `#emulator-form-modal`

- [ ] **Step 1: Escrever o teste de renderização**

Primeiro, o helper. Acrescentar no fim de `internal/handlers/render_pages_test.go`:

```go
// renderizarPagina renderiza um template com o contexto dado e devolve o
// HTML. Complementa TestRenderTodasAsPaginas, que só confere que a página
// fecha; aqui os testes olham o conteúdo.
func renderizarPagina(t *testing.T, pagina string, dados gin.H) string {
	t.Helper()

	h := &Handler{templates: buildTemplateCache(), appVersion: "teste"}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.renderPage(c, pagina, http.StatusOK, dados)
	return rec.Body.String()
}

// renderizarDevices monta o contexto mínimo que devices.html exige.
func renderizarDevices(t *testing.T, devices []map[string]interface{}) string {
	t.Helper()
	return renderizarPagina(t, "devices.html", gin.H{
		"devices":       devices,
		"page":          1,
		"total_pages":   1,
		"per_page":      10,
		"page_range":    []int{1},
		"counter_cards": FleetCounts{}.toMap(),
		"filters":       map[string]string{"id": "", "name": "", "port": ""},
	})
}
```

**Acrescentar `"source": "wxs"` ao mapa do dispositivo do caso `devices.html` em `TestRenderTodasAsPaginas`** (linha ~24). O template passa a usar `{{ if eq .source "manual" }}`, e `eq` contra uma chave ausente num mapa aborta a renderização — sem isso o teste que já existe quebra.

Agora os casos novos, no mesmo arquivo:

```go
func TestDevicesHTMLMostraOrigem(t *testing.T) {
	html := renderizarDevices(t, []map[string]interface{}{
		{
			"lc_id": 900001, "name": "lab-4000", "ip_address": "127.0.0.1",
			"port": 4000, "log_enabled": 0, "model": "Dahua", "status": "stopped",
			"enabled": 1, "interval": 10, "total": 0, "local_auth": "standalone",
			"source": "manual",
		},
	})

	if !strings.Contains(html, "Manual") {
		t.Error("quero o badge de origem manual na grade")
	}
	if !strings.Contains(html, "device-remove") {
		t.Error("quero o botão de remover em dispositivo manual")
	}
}

func TestDevicesHTMLNaoOfereceRemoverEmDispositivoDoWXS(t *testing.T) {
	html := renderizarDevices(t, []map[string]interface{}{
		{
			"lc_id": 17, "name": "Portaria", "ip_address": "10.0.0.7",
			"port": 7070, "log_enabled": 0, "model": "Hikvision", "status": "stopped",
			"enabled": 1, "interval": 10, "total": 3, "local_auth": "online",
			"source": "wxs",
		},
	})

	if strings.Contains(html, "device-remove") {
		t.Error("dispositivo do W-Access não pode oferecer remoção")
	}
}
```

`renderizarDevices` é um helper a criar no mesmo arquivo, montando o contexto mínimo que `devices.html` exige (`devices`, `filters`, `per_page`, `page`, `total_pages`, `page_range`, `counter_cards`) e devolvendo o HTML renderizado — espelhando o que os testes existentes do arquivo já fazem.

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/handlers/ -run TestDevicesHTML -v`
Expected: FAIL — o template ainda não tem coluna de origem

- [ ] **Step 3: Acrescentar a coluna e as ações ao template**

Em `assets/web/templates/devices.html`, no `<thead>`, depois de `<th>Modelo</th>`:

```html
        <th>Origem</th>
```

No `<tbody>`, na mesma posição relativa, depois de `<td>{{ .model }}</td>`:

```html
        <td>
          <!-- Origem manda em quais ações a linha oferece: o que veio do
               W-Access é sobrescrito pelo próximo sync, então editar ali
               seria uma promessa que o serviço não cumpre. -->
          {{ if eq .source "manual" }}
          <span class="badge badge--manual">Manual</span>
          {{ else }}
          <span class="badge badge--wxs">W-Access</span>
          {{ end }}
        </td>
```

Dentro de `.row-actions`, depois do botão de detalhes:

```html
            {{ if eq .source "manual" }}
            <button type="button" class="btn btn--sm device-edit"
                    data-id="{{ .lc_id }}" title="Editar"
                    {{ if eq .status "running" }}disabled{{ end }}>
              <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#settings"></use></svg>
              <span class="visually-hidden">Editar {{ .name }}</span>
            </button>
            <button type="button" class="btn btn--sm btn--halt device-remove"
                    data-id="{{ .lc_id }}" data-name="{{ .name }}" title="Remover">
              <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#trash"></use></svg>
              <span class="visually-hidden">Remover {{ .name }}</span>
            </button>
            {{ end }}
```

O `colspan` da linha de estado vazio sobe de 11 para 12.

Conferir se `#settings` e `#trash` existem em `assets/web/static/icons.svg`:

Run: `grep -o 'id="[a-z-]*"' assets/web/static/icons.svg`

Se algum faltar, usar um id existente da lista devolvida — nunca referenciar um símbolo inexistente, que renderiza um botão vazio.

- [ ] **Step 4: Estilizar os badges de origem**

Em `assets/web/static/css/components.css`, junto das outras regras `.badge`:

```css
/* Origem do dispositivo. Cores discretas: a informação é contexto, não
   alerta — o estado (ativo/parado) é que precisa saltar. */
.badge--manual {
  background: var(--surface-2);
  color: var(--text-2);
}

.badge--wxs {
  background: var(--surface-2);
  color: var(--text-2);
  font-style: italic;
}
```

Conferir antes os nomes reais das custom properties:

Run: `grep -n '\-\-surface\|--text' assets/web/static/css/tokens.css | head -20`

Usar os tokens que existirem; não inventar nomes novos.

- [ ] **Step 5: Ligar remover e editar no JS**

Em `assets/web/static/js/devices.js`, no mesmo bloco onde os listeners de `.device-start` e `.device-stop` são registrados:

```js
  // Remover é irreversível e leva junto cartões e faces cadastrados
  // naquele emulador — a confirmação precisa dizer isso, não só "tem
  // certeza?".
  document.querySelectorAll('.device-remove').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const id = btn.dataset.id;
      const nome = btn.dataset.name || id;
      const ok = window.confirm(
        `Remover o emulador "${nome}"?\n\n` +
        'Os cartões, faces e usuários cadastrados nele também serão apagados. ' +
        'Não há como desfazer.'
      );
      if (!ok) return;

      btn.disabled = true;
      try {
        const resp = await fetch(`/api/emulators/${id}`, { method: 'DELETE' });
        const corpo = await resp.json();
        if (!resp.ok) {
          showToast(corpo.error || 'Falha ao remover o emulador', 'error');
          btn.disabled = false;
          return;
        }
        showToast(`Emulador ${nome} removido`, 'success');
        document.getElementById(`device-${id}`)?.remove();
      } catch (err) {
        showToast(`Falha ao remover: ${err.message}`, 'error');
        btn.disabled = false;
      }
    });
  });

  document.querySelectorAll('.device-edit').forEach((btn) => {
    btn.addEventListener('click', () => {
      const linha = document.getElementById(`device-${btn.dataset.id}`);
      abrirFormularioEmulador({
        id: btn.dataset.id,
        name: linha.querySelector('.device-name-cell').textContent.trim(),
        model: linha.children[3].textContent.trim(),
        port: Number(linha.querySelector('td.num').textContent.trim()),
      });
    });
  });
```

`abrirFormularioEmulador` é definida na Task 8. Nesta tarefa, declarar o stub no topo do arquivo para o listener não quebrar caso a Task 8 ainda não tenha rodado:

```js
// Definida em Task 8; o stub evita ReferenceError se o modal ainda não
// existir na página.
window.abrirFormularioEmulador = window.abrirFormularioEmulador || function () {
  showToast('Formulário de emulador indisponível', 'error');
};
```

Conferir o nome real da função de toast antes:

Run: `grep -n 'function\|export\|window\.' assets/web/static/js/toast.js | head`

- [ ] **Step 6: Rodar os testes**

Run: `go test ./internal/handlers/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add assets/web/templates/devices.html assets/web/static/js/devices.js assets/web/static/css/components.css internal/handlers/render_pages_test.go
git commit -m "feat(ui): origem do dispositivo na grade, com editar e remover em manuais"
```

---

### Task 8: Modais de criação — um e em lote

**Files:**
- Modify: `assets/web/templates/devices.html`
- Create: `assets/web/static/js/emulator-form.js`
- Modify: `internal/handlers/render_pages_test.go`

**Interfaces:**
- Consumes: `POST /api/emulators`, `POST /api/emulators/range`, `PUT /api/emulators/:id` (Task 6)
- Produces: `window.abrirFormularioEmulador(dados)` — sem `dados` abre em modo criação, com `dados.id` abre em modo edição

- [ ] **Step 1: Escrever o teste de renderização dos botões**

Em `internal/handlers/render_pages_test.go`:

```go
func TestDevicesHTMLTemBotoesDeCadastro(t *testing.T) {
	html := renderizarDevices(t, nil)

	for _, id := range []string{"new-emulator", "new-emulator-range", "emulator-form-modal"} {
		if !strings.Contains(html, id) {
			t.Errorf("quero o elemento %q na página", id)
		}
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/handlers/ -run TestDevicesHTMLTemBotoes -v`
Expected: FAIL

- [ ] **Step 3: Acrescentar os botões e o modal ao template**

Em `assets/web/templates/devices.html`, dentro de `.page-head__actions`, antes de `#start-selected`:

```html
    <button type="button" class="btn btn--action" id="new-emulator">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#plus"></use></svg>
      Novo emulador
    </button>
    <button type="button" class="btn" id="new-emulator-range">
      <svg class="icon" aria-hidden="true"><use href="/static/icons.svg#devices"></use></svg>
      Criar em lote
    </button>
```

No fim do bloco `{{ define "content" }}`, antes do `{{ end }}`:

```html
<!-- Um modal só, dois modos. O lote troca "Porta" por "Porta inicial /
     final" e "Nome" por "Prefixo": o resto dos campos é idêntico, e dois
     formulários separados seriam a mesma coisa duplicada. -->
<dialog class="modal" id="emulator-form-modal">
  <form method="dialog" class="panel" id="emulator-form">
    <div class="panel__head" id="emulator-form-title">Novo emulador</div>

    <div class="panel__body">
      <div class="field" id="field-name">
        <label class="field__label" for="emulator-name">Nome</label>
        <input class="input" type="text" id="emulator-name" required>
      </div>

      <div class="field" id="field-prefix" hidden>
        <label class="field__label" for="emulator-prefix">Prefixo do nome</label>
        <input class="input" type="text" id="emulator-prefix"
               placeholder="lab">
        <p class="field__help">Cada emulador recebe o nome
          <code>prefixo-porta</code>, por exemplo <code>lab-4000</code>.</p>
      </div>

      <div class="field">
        <label class="field__label" for="emulator-model">Modelo</label>
        <select class="select" id="emulator-model">
          <option value="Hikvision">Hikvision</option>
          <option value="Dahua">Dahua</option>
        </select>
      </div>

      <div class="field">
        <label class="field__label" for="emulator-ip">Endereço IP</label>
        <input class="input input--mono" type="text" id="emulator-ip"
               value="127.0.0.1">
        <p class="field__help">Só aparece nas respostas que o emulador
          devolve. O servidor escuta em todas as interfaces.</p>
      </div>

      <div class="field" id="field-port">
        <label class="field__label" for="emulator-port">Porta</label>
        <input class="input input--mono" type="number" id="emulator-port"
               min="1" max="65535" value="4000">
      </div>

      <div class="field" id="field-port-range" hidden>
        <label class="field__label" for="emulator-port-start">Porta inicial</label>
        <input class="input input--mono" type="number" id="emulator-port-start"
               min="1" max="65535" value="4000">
        <label class="field__label" for="emulator-port-end">Porta final</label>
        <input class="input input--mono" type="number" id="emulator-port-end"
               min="1" max="65535" value="4009">
        <p class="field__help">Máximo de 1000 portas por lote.</p>
      </div>

      <div class="field">
        <label class="field__label" for="emulator-interval">Intervalo de eventos (s)</label>
        <input class="input input--mono" type="number" id="emulator-interval"
               min="0" value="10">
      </div>

      <div class="field field--inline">
        <input class="check" type="checkbox" id="emulator-enabled" checked>
        <label class="field__label" for="emulator-enabled">Habilitado</label>
      </div>

      <div class="field field--inline" id="field-autostart">
        <input class="check" type="checkbox" id="emulator-autostart">
        <label class="field__label" for="emulator-autostart">Iniciar após criar</label>
      </div>

      <p class="panel__note" id="emulator-form-error" hidden></p>

      <div class="filters__actions">
        <button type="button" class="btn btn--action" id="emulator-form-save">Salvar</button>
        <button type="button" class="btn" id="emulator-form-cancel">Cancelar</button>
      </div>
    </div>
  </form>
</dialog>
```

No bloco `{{ define "additional_js" }}` de `devices.html`, antes de `devices.js`:

```html
<script src="/static/js/emulator-form.js"></script>
```

A ordem importa: `devices.js` referencia `window.abrirFormularioEmulador`.

- [ ] **Step 4: Implementar o JS do formulário**

Criar `assets/web/static/js/emulator-form.js`:

```js
// Formulário de emulador: criação de um, criação em lote e edição.
// Um modal só, com os campos que diferem trocados por modo — a alternativa
// eram três formulários com os mesmos seis campos repetidos.

(function () {
  const modal = document.getElementById('emulator-form-modal');
  if (!modal) return;

  const el = (id) => document.getElementById(id);
  const titulo = el('emulator-form-title');
  const erro = el('emulator-form-error');

  let modo = 'criar'; // 'criar' | 'lote' | 'editar'
  let idEmEdicao = null;

  function mostrarCampos() {
    const lote = modo === 'lote';
    el('field-name').hidden = lote;
    el('field-prefix').hidden = !lote;
    el('field-port').hidden = lote;
    el('field-port-range').hidden = !lote;
    // Editar exige o emulador parado, então "iniciar após criar" não se
    // aplica: o campo sumiria de qualquer jeito no submit.
    el('field-autostart').hidden = modo === 'editar';
  }

  function limparErro() {
    erro.hidden = true;
    erro.textContent = '';
  }

  function mostrarErro(texto) {
    erro.textContent = texto;
    erro.hidden = false;
  }

  function abrir(dados) {
    limparErro();

    if (dados && dados.id) {
      modo = 'editar';
      idEmEdicao = dados.id;
      titulo.textContent = `Editar emulador ${dados.id}`;
      el('emulator-name').value = dados.name || '';
      el('emulator-model').value = dados.model || 'Hikvision';
      el('emulator-port').value = dados.port || 4000;
    } else if (dados && dados.lote) {
      modo = 'lote';
      idEmEdicao = null;
      titulo.textContent = 'Criar emuladores em lote';
    } else {
      modo = 'criar';
      idEmEdicao = null;
      titulo.textContent = 'Novo emulador';
    }

    mostrarCampos();
    modal.showModal();
  }

  window.abrirFormularioEmulador = abrir;

  el('new-emulator')?.addEventListener('click', () => abrir(null));
  el('new-emulator-range')?.addEventListener('click', () => abrir({ lote: true }));
  el('emulator-form-cancel').addEventListener('click', () => modal.close());

  function corpoComum() {
    return {
      model: el('emulator-model').value,
      ip_address: el('emulator-ip').value.trim(),
      event_interval: Number(el('emulator-interval').value),
      enabled: el('emulator-enabled').checked,
      auto_start: el('emulator-autostart').checked,
    };
  }

  function requisicao() {
    if (modo === 'lote') {
      return {
        url: '/api/emulators/range',
        method: 'POST',
        body: {
          ...corpoComum(),
          name_prefix: el('emulator-prefix').value.trim(),
          port_start: Number(el('emulator-port-start').value),
          port_end: Number(el('emulator-port-end').value),
        },
      };
    }

    const body = {
      ...corpoComum(),
      name: el('emulator-name').value.trim(),
      port: Number(el('emulator-port').value),
    };

    if (modo === 'editar') {
      delete body.auto_start;
      return { url: `/api/emulators/${idEmEdicao}`, method: 'PUT', body };
    }
    return { url: '/api/emulators', method: 'POST', body };
  }

  el('emulator-form-save').addEventListener('click', async () => {
    limparErro();
    const { url, method, body } = requisicao();
    const salvar = el('emulator-form-save');
    salvar.disabled = true;

    try {
      const resp = await fetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      const corpo = await resp.json();

      if (!resp.ok) {
        // O erro de porta traz a lista crua; repeti-la é o que diz ao
        // operador qual porta trocar.
        let msg = corpo.error || 'Falha ao salvar';
        if (Array.isArray(corpo.conflicts) && corpo.conflicts.length > 0) {
          msg = `Portas já em uso: ${corpo.conflicts.join(', ')}`;
        }
        mostrarErro(msg);
        salvar.disabled = false;
        return;
      }

      modal.close();
      window.location.reload();
    } catch (e) {
      mostrarErro(`Falha de rede: ${e.message}`);
      salvar.disabled = false;
    }
  });
})();
```

- [ ] **Step 5: Rodar os testes**

Run: `go test ./internal/handlers/ -v`
Expected: PASS

- [ ] **Step 6: Conferir o modal num navegador**

Se houver PostgreSQL disponível, subir o serviço, abrir `http://localhost:8080/`, clicar em "Novo emulador" e em "Criar em lote", e confirmar que os campos trocam entre os modos e que o erro de porta em conflito aparece dentro do modal.

Sem banco, registrar no commit que a verificação visual ficou pendente.

- [ ] **Step 7: Commit**

```bash
git add assets/web/templates/devices.html assets/web/static/js/emulator-form.js internal/handlers/render_pages_test.go
git commit -m "feat(ui): modal de criação de emulador, avulso e em lote"
```

---

### Task 9: Toggle de sincronização na tela de configurações

**Files:**
- Modify: `assets/web/templates/settings.html`
- Modify: `assets/web/static/js/settings.js`
- Modify: `assets/web/templates/sidebar.html` ou `header.html` (só se o botão de refresh do W-Access morar lá)
- Modify: `internal/handlers/render_pages_test.go`

**Interfaces:**
- Consumes: `sync_enabled` no contexto de `settings.html` (Task 4); `POST /api/settings/sync` (Task 4)
- Produces: `#sync-enabled` na tela de configurações

- [ ] **Step 1: Escrever o teste de renderização**

Em `internal/handlers/render_pages_test.go`:

```go
func TestSettingsHTMLTemToggleDeSync(t *testing.T) {
	html := renderizarSettings(t, gin.H{
		"wxs_settings": &database.WxsSettings{Host: "10.0.0.2", Port: 1433},
		"sync_enabled": true,
	})

	if !strings.Contains(html, "sync-enabled") {
		t.Error("quero o toggle de sincronização na tela")
	}
	if !strings.Contains(html, "checked") {
		t.Error("sync ligado tem que vir marcado")
	}
}
```

`renderizarSettings` é um helper a criar no mesmo arquivo, sobre o `renderizarPagina` da Task 7:

```go
func renderizarSettings(t *testing.T, dados gin.H) string {
	t.Helper()
	return renderizarPagina(t, "settings.html", dados)
}
```

O import de `GoFacialEmulator/internal/database` entra no arquivo junto com este teste.

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/handlers/ -run TestSettingsHTML -v`
Expected: FAIL

- [ ] **Step 3: Acrescentar o toggle ao template**

Em `assets/web/templates/settings.html`, dentro do `.panel__body` do formulário do W-Access, antes de `.filters__actions`:

```html
    <!-- O vínculo com o Invenzi é opcional. Desligado, o serviço vive só
         dos emuladores cadastrados aqui, e nada do W-Access apaga o que
         foi cadastrado à mão. -->
    <div class="field field--inline">
      <input class="check" type="checkbox" id="sync-enabled"
             {{ if .sync_enabled }}checked{{ end }}>
      <label class="field__label" for="sync-enabled">
        Sincronizar dispositivos com o W-Access
      </label>
    </div>
    <p class="field__help">
      Desligado, o serviço não busca nem remove dispositivos do W-Access.
      Emuladores cadastrados aqui nunca são afetados pela sincronização.
    </p>
```

- [ ] **Step 4: Ligar o toggle no JS**

Em `assets/web/static/js/settings.js`, no fim do arquivo:

```js
// O toggle grava sozinho, sem passar pelo botão Salvar do formulário: ele
// não é credencial, e exigir "Salvar" para uma chave booleana faria o
// operador achar que precisa redigitar a senha para desligar o sync.
const syncToggle = document.getElementById('sync-enabled');
if (syncToggle) {
  syncToggle.addEventListener('change', async () => {
    const ligado = syncToggle.checked;
    syncToggle.disabled = true;

    try {
      const resp = await fetch('/api/settings/sync', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: ligado }),
      });
      const corpo = await resp.json();

      if (!resp.ok) {
        syncToggle.checked = !ligado;
        showToast(corpo.error || 'Falha ao gravar a configuração', 'error');
        return;
      }

      showToast(
        ligado ? 'Sincronização com o W-Access ligada'
               : 'Sincronização com o W-Access desligada',
        'success'
      );
    } catch (e) {
      syncToggle.checked = !ligado;
      showToast(`Falha de rede: ${e.message}`, 'error');
    } finally {
      syncToggle.disabled = false;
    }
  });
}
```

Conferir antes se `settings.js` já tem acesso à função de toast:

Run: `grep -n 'toast' assets/web/templates/settings.html assets/web/templates/base.html assets/web/static/js/settings.js`

Se `toast.js` não estiver carregado nessa página, acrescentar o `<script>` ao `additional_js` de `settings.html`.

- [ ] **Step 5: Desabilitar o refresh quando o sync está desligado**

Run: `grep -rn 'refresh' assets/web/templates/ assets/web/static/js/ | grep -v node_modules`

Onde estiver o botão que dispara `/refresh` ou `/api/emulators/control/refresh`, tratar a resposta 409 mostrando a mensagem do corpo via toast, em vez de um erro genérico:

```js
      if (resp.status === 409) {
        const corpo = await resp.json();
        showToast(corpo.error || 'Sincronização desligada', 'warn');
        return;
      }
```

- [ ] **Step 6: Rodar os testes**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add assets/web/templates/settings.html assets/web/static/js/settings.js internal/handlers/render_pages_test.go
git commit -m "feat(ui): toggle de sincronização com o W-Access"
```

---

### Task 10: Documentação

`PORTAS-EMULADORES.md` afirma hoje, em negrito, o contrário do que o serviço passa a fazer. Documento errado é pior que documento ausente.

**Files:**
- Modify: `PORTAS-EMULADORES.md:16-50`
- Modify: `README.md`

- [ ] **Step 1: Reescrever a origem das portas**

Em `PORTAS-EMULADORES.md`, substituir a seção "**1. Origem das Portas**" e o fluxograma que a segue por:

```markdown
### **1. Origem das Portas**

As portas dos emuladores têm duas origens possíveis, e a escolha é do
operador:

**Cadastro no próprio serviço.** Pela tela de dispositivos ou pela API
(`POST /api/emulators` e `POST /api/emulators/range`), o operador define a
porta de cada emulador. É o caminho para testar com qualquer sistema de
controle de acesso, ou sem sistema nenhum.

**Sincronização com o W-Access.** Com o vínculo ligado em
**Configurações → Sincronizar dispositivos com o W-Access**, o serviço lê
`CfgHWLocalControllers.BaseCommPort` e cria um emulador por controlador
cujo `LocalControllerDescription` comece com `emulator`.

```
Cadastro manual (UI ou API) ──┐
                              ├──> service.devices.port ──> emulador na porta X
WXS: LocalControllers.Port ───┘        (coluna source diz qual origem)
```

Os dois convivem. A sincronização só cria, atualiza e remove dispositivos
de origem `wxs`; emuladores cadastrados à mão nunca são tocados por ela.
Desligar o vínculo não apaga nada — apenas para de buscar.

### **Limite do lote**

O cadastro em lote aceita no máximo **1000 portas** de uma vez, que é a
largura do range publicado no `docker-compose.yml` (4000-4999). Um lote
fora dessa faixa cadastra emuladores que, sob Docker, sobem mas nascem
inalcançáveis de fora do contêiner.
```

- [ ] **Step 2: Documentar a API no README**

Em `README.md`, acrescentar uma seção depois da que descreve o serviço:

````markdown
## API de emuladores

O serviço gerencia os próprios emuladores. Nenhuma rota exige
autenticação — é ferramenta de laboratório, pensada para rede local.

### Criar um

```bash
curl -X POST localhost:8080/api/emulators \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "lab-01",
        "model": "Hikvision",
        "ip_address": "192.168.1.50",
        "port": 4001,
        "event_interval": 10,
        "enabled": true,
        "auto_start": false
      }'
```

`model` aceita `"Hikvision"` ou `"Dahua"`. Omitindo campos: `ip_address`
vira `127.0.0.1`, `event_interval` vira `10`, `enabled` vira `true` e
`auto_start` vira `false`.

### Criar em lote

```bash
curl -X POST localhost:8080/api/emulators/range \
  -H 'Content-Type: application/json' \
  -d '{
        "name_prefix": "lab",
        "model": "Dahua",
        "ip_address": "192.168.1.50",
        "port_start": 4000,
        "port_end": 4049,
        "auto_start": false
      }'
```

Gera um emulador por porta, nomeados `lab-4000`, `lab-4001` e assim por
diante. Máximo de 1000 portas por lote. Se qualquer porta do intervalo já
estiver cadastrada, a requisição falha com `400` listando os conflitos e
**nada é criado**:

```json
{ "error": "portas já usadas por outros emuladores: [4003 4004]",
  "conflicts": [4003, 4004] }
```

### Listar, editar e remover

```bash
curl localhost:8080/api/emulators

curl -X PUT localhost:8080/api/emulators/900001 \
  -H 'Content-Type: application/json' \
  -d '{"name":"lab-01","model":"Dahua","port":4001}'

curl -X DELETE localhost:8080/api/emulators/900001
```

`PUT` é substituição total dos campos editáveis — campo ausente cai no
padrão, não mantém o valor anterior.

Remover apaga junto os cartões, faces e usuários cadastrados naquele
emulador. Não há como desfazer.

Dois casos devolvem `409`: editar ou remover um dispositivo que veio do
W-Access (a verdade dele mora lá, e o próximo sync sobrescreveria), e
editar um emulador que está rodando (pare antes).

### Controle e sincronização

`/api/devices/{id}/start`, `/stop`, `/settings` e `/mode` continuam
valendo para emuladores de qualquer origem.
`/api/emulators/control/{start,stop,refresh}` age sobre a frota inteira;
`refresh` devolve `409` quando a sincronização com o W-Access está
desligada.
````

- [ ] **Step 3: Conferir os textos**

Run: `grep -n 'NÃO são definidas' PORTAS-EMULADORES.md`
Expected: nenhuma linha — a afirmação antiga não pode ter sobrado

- [ ] **Step 4: Commit**

```bash
git add PORTAS-EMULADORES.md README.md
git commit -m "docs: origem das portas e API de CRUD de emuladores"
```

---

## Verificação final

Depois da Task 10, com PostgreSQL disponível:

- [ ] `go build ./... && go vet ./... && go test ./...` — tudo verde
- [ ] Subir o serviço sobre um banco **já existente**, com dispositivos gravados: conferir no log `Migração V002 aplicada` e que nenhum dispositivo sumiu (`SELECT count(*), source FROM service.devices GROUP BY source`)
- [ ] Subir sobre banco vazio: validator cria o baseline, runner aplica a V002, `/` abre sem dispositivo nenhum e o botão "Novo emulador" funciona
- [ ] Criar um lote de 10 portas, iniciar, e conferir que respondem: `curl -s localhost:4000/ISAPI/System/deviceInfo`
- [ ] Com o sync desligado, chamar `/refresh` e confirmar o `409`
- [ ] Com o sync ligado e W-Access acessível, rodar um refresh e confirmar que os 10 emuladores manuais continuam na lista
- [ ] Remover um emulador manual e conferir que as tabelas `emulator.*` não têm mais linha com aquele `device_id`
