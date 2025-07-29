package database

import (
	"context"
	"fmt"

	"GoFacialEmulator/internal/config"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// EmulatorDB gerencia operações globais no banco do emulador
type EmulatorDB struct {
	pool   *pgxpool.Pool
	schema string
}

// NewEmulatorDB cria uma nova instância do EmulatorDB
func NewEmulatorDB(cfg config.DatabaseConfig) (*EmulatorDB, error) {
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao PostgreSQL: %w", err)
	}

	return &EmulatorDB{
		pool:   pool,
		schema: cfg.Schema,
	}, nil
}

// Close fecha a conexão
func (db *EmulatorDB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Implementar interface DBInterface
func (db *EmulatorDB) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, query, args...)
}

func (db *EmulatorDB) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, query, args...)
}

func (db *EmulatorDB) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, query, args...)
}

func (db *EmulatorDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return db.pool.Begin(ctx)
}

func (db *EmulatorDB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Transaction executa uma função dentro de uma transação
func (db *EmulatorDB) Transaction(ctx context.Context, txFunc func(tx pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := txFunc(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ====================== DEVICE SETTINGS (Global para todos os fabricantes) ======================

// GetDeviceSettings obtém uma configuração do dispositivo
func (db *EmulatorDB) GetDeviceSettings(ctx context.Context, deviceID int, cfgID string) (string, error) {
	var value string
	err := db.QueryRow(ctx,
		"SELECT value FROM emulator.device_settings WHERE device_id = $1 AND cfg_id = $2",
		deviceID, cfgID).Scan(&value)
	return value, err
}

// SetDeviceSettings define uma configuração do dispositivo
func (db *EmulatorDB) SetDeviceSettings(ctx context.Context, deviceID int, cfgID, value string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO emulator.device_settings (device_id, cfg_id, value) 
		 VALUES ($1, $2, $3) 
		 ON CONFLICT (device_id, cfg_id) DO UPDATE SET value = $3, updated_at = NOW()`,
		deviceID, cfgID, value)
	return err
}

// ====================== GENERAL DEVICE OPERATIONS (Global) ======================

// GetTotalUsers retorna o total de usuários para um dispositivo (genérico)
func (db *EmulatorDB) GetTotalUsers(ctx context.Context, deviceType string, deviceID int) (int, error) {
	var count int
	var query string

	switch deviceType {
	case "Hikvision":
		query = "SELECT COUNT(*) FROM emulator.hikvision_users WHERE device_id = $1"
	case "Dahua":
		query = "SELECT COUNT(*) FROM emulator.dahua_users WHERE device_id = $1"
	default:
		return 0, fmt.Errorf("unsupported device type: %s", deviceType)
	}

	err := db.QueryRow(ctx, query, deviceID).Scan(&count)
	return count, err
}
