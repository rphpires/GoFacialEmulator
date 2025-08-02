package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"GoFacialEmulator/internal/config"

	"github.com/jackc/pgx/v4/pgxpool"
)

// MigrationManager gerencia as migrações do banco de dados (DEPRECADO)
// Use ValidateDatabaseOnStartup() ao invés disso
type MigrationManager struct {
	pool   *pgxpool.Pool
	schema string
}

// NewMigrationManager cria um novo gerenciador de migrações (DEPRECADO)
func NewMigrationManager(cfg config.DatabaseConfig) (*MigrationManager, error) {
	log.Println("⚠️  MigrationManager está deprecado. Use ValidateDatabaseOnStartup() ao invés disso.")

	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao PostgreSQL: %w", err)
	}

	return &MigrationManager{
		pool:   pool,
		schema: cfg.Schema,
	}, nil
}

// Close fecha a conexão com o banco de dados
func (m *MigrationManager) Close() {
	if m.pool != nil {
		m.pool.Close()
	}
}

// InitializeDatabase inicializa o banco de dados (DEPRECADO)
// Use ValidateDatabaseOnStartup() ao invés disso
func InitializeDatabase(cfg config.DatabaseConfig) error {
	log.Println("⚠️  InitializeDatabase está deprecado. Use ValidateDatabaseOnStartup() ao invés disso.")

	// Usar o novo validador
	return ValidateDatabaseOnStartup(cfg)
}

// CreateMigrationsTable (DEPRECADO - não é mais necessário)
func (m *MigrationManager) CreateMigrationsTable(ctx context.Context) error {
	log.Println("⚠️  CreateMigrationsTable não é mais necessário com o novo sistema")
	return nil
}

// GetAppliedMigrations (DEPRECADO - não é mais necessário)
func (m *MigrationManager) GetAppliedMigrations(ctx context.Context) (map[string]time.Time, error) {
	log.Println("⚠️  GetAppliedMigrations não é mais necessário com o novo sistema")
	return make(map[string]time.Time), nil
}

// RunMigrations (DEPRECADO - use ValidateDatabaseOnStartup)
func (m *MigrationManager) RunMigrations(ctx context.Context) error {
	log.Println("⚠️  RunMigrations está deprecado. O novo sistema valida e recria automaticamente.")
	return nil
}

// ExecuteMigration (DEPRECADO)
func (m *MigrationManager) ExecuteMigration(ctx context.Context, version, content string) error {
	log.Println("⚠️  ExecuteMigration não é mais necessário com o novo sistema")
	return nil
}
