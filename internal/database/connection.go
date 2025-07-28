package database

import (
	"context"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// DBInterface define operações básicas de banco
type DBInterface interface {
	Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PostgresPool gerencia pool de conexões
type PostgresPool struct {
	pool *pgxpool.Pool
}

func NewPostgresPool(dsn string) (*PostgresPool, error) {
	// Implementação do pool
}
