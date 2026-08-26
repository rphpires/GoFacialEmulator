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
func (t *txFalsa) LargeObjects() pgx.LargeObjects                               { return pgx.LargeObjects{} }
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
	tx        *txFalsa
	execs     []string
	execArgs  [][]interface{}
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
