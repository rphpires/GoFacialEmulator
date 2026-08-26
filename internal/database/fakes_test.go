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

func (r *rowsFalsas) Close()                                        {}
func (r *rowsFalsas) Err() error                                    { return r.err }
func (r *rowsFalsas) CommandTag() pgconn.CommandTag                 { return pgconn.CommandTag{} }
func (r *rowsFalsas) FieldDescriptions() []pgproto3.FieldDescription { return nil }
func (r *rowsFalsas) Values() ([]interface{}, error)                { return nil, nil }
func (r *rowsFalsas) RawValues() [][]byte                           { return nil }

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

func (t *txFalsa) Begin(ctx context.Context) (pgx.Tx, error)                 { return t, nil }
func (t *txFalsa) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error { return f(t) }
func (t *txFalsa) Commit(ctx context.Context) error                         { t.comitou = true; return nil }
func (t *txFalsa) Rollback(ctx context.Context) error                       { t.desfez = true; return nil }
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

	// queryRowSQLs acumula o texto de cada QueryRow recebido, na ordem —
	// para testes que precisam confirmar que uma cláusula específica
	// (ex.: "WHERE host <> ''") está de fato na query emitida, já que
	// linhaFalsa/rowsFalsas não filtram nada de verdade.
	queryRowSQLs []string
}

func (d *dbFalso) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if d.queryRows == nil {
		return &rowsFalsas{}, nil
	}
	return d.queryRows, nil
}

func (d *dbFalso) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	d.queryRowSQLs = append(d.queryRowSQLs, sql)
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
