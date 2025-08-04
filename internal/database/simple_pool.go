// internal/database/simple_pool.go
package database

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// SimpleOptimizedPool - pool único com configuração otimizada
type SimpleOptimizedPool struct {
	pool *pgxpool.Pool
	mu   sync.RWMutex
}

func NewSimpleOptimizedPool(dbURL string) (*SimpleOptimizedPool, error) {
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, err
	}

	// Configuração otimizada para 300 emuladores
	config.MinConns = 5
	config.MaxConns = 25 // Muito menor que antes!
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 15 * time.Minute
	config.HealthCheckPeriod = 1 * time.Minute

	// Otimizações simples
	config.ConnConfig.PreferSimpleProtocol = true
	config.ConnConfig.RuntimeParams = map[string]string{
		"application_name": "facial_emulator",
	}

	pool, err := pgxpool.ConnectConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}

	return &SimpleOptimizedPool{pool: pool}, nil
}

func (p *SimpleOptimizedPool) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return p.pool.Query(ctx, query, args...)
}

func (p *SimpleOptimizedPool) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return p.pool.QueryRow(ctx, query, args...)
}

func (p *SimpleOptimizedPool) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return p.pool.Exec(ctx, query, args...)
}

func (p *SimpleOptimizedPool) Begin(ctx context.Context) (pgx.Tx, error) {
	return p.pool.Begin(ctx)
}

func (p *SimpleOptimizedPool) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *SimpleOptimizedPool) Close() {
	p.pool.Close()
}
