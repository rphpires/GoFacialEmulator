package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
	"time"

	"GoFacialEmulator/internal/config"

	"github.com/jackc/pgx/v4/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationManager gerencia as migrações do banco de dados
type MigrationManager struct {
	pool   *pgxpool.Pool
	schema string
}

// NewMigrationManager cria um novo gerenciador de migrações
func NewMigrationManager(cfg config.DatabaseConfig) (*MigrationManager, error) {
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

// CreateMigrationsTable cria a tabela de migrações se não existir
func (m *MigrationManager) CreateMigrationsTable(ctx context.Context) error {
	// Certificar que o schema existe
	_, err := m.pool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", m.schema))
	if err != nil {
		return fmt.Errorf("erro ao criar schema: %w", err)
	}

	// Criar tabela de migrações
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.migrations (
			id SERIAL PRIMARY KEY,
			version VARCHAR(255) NOT NULL UNIQUE,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`, m.schema)

	_, err = m.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("erro ao criar tabela de migrações: %w", err)
	}

	return nil
}

// GetAppliedMigrations retorna a lista de migrações já aplicadas
func (m *MigrationManager) GetAppliedMigrations(ctx context.Context) (map[string]time.Time, error) {
	query := fmt.Sprintf("SELECT version, applied_at FROM %s.migrations ORDER BY version", m.schema)

	rows, err := m.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter migrações aplicadas: %w", err)
	}
	defer rows.Close()

	migrations := make(map[string]time.Time)
	for rows.Next() {
		var version string
		var appliedAt time.Time

		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, fmt.Errorf("erro ao ler migração: %w", err)
		}

		migrations[version] = appliedAt
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar migrações: %w", err)
	}

	return migrations, nil
}

// MarkMigrationAsApplied marca uma migração como aplicada
func (m *MigrationManager) MarkMigrationAsApplied(ctx context.Context, version string) error {
	query := fmt.Sprintf("INSERT INTO %s.migrations (version) VALUES ($1)", m.schema)

	_, err := m.pool.Exec(ctx, query, version)
	if err != nil {
		return fmt.Errorf("erro ao marcar migração %s como aplicada: %w", version, err)
	}

	return nil
}

// RunMigrations executa as migrações pendentes
func (m *MigrationManager) RunMigrations(ctx context.Context) error {
	// Criar tabela de migrações se não existir
	if err := m.CreateMigrationsTable(ctx); err != nil {
		return err
	}

	// Obter migrações já aplicadas
	appliedMigrations, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Listar arquivos de migração
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("erro ao ler diretório de migrações: %w", err)
	}

	// Ordenar migrações
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Executar migrações pendentes
	for _, filename := range migrationFiles {
		// Extrair versão da migração (formato: V001_nome.sql)
		parts := strings.SplitN(filename, "_", 2)
		if len(parts) != 2 {
			log.Printf("Formato de arquivo de migração inválido: %s, pulando...", filename)
			continue
		}
		version := parts[0]

		// Verificar se a migração já foi aplicada
		if _, exists := appliedMigrations[version]; exists {
			log.Printf("Migração %s já aplicada, pulando...", version)
			continue
		}

		// Ler conteúdo da migração
		content, err := fs.ReadFile(migrationsFS, fmt.Sprintf("migrations/%s", filename))
		if err != nil {
			return fmt.Errorf("erro ao ler arquivo de migração %s: %w", filename, err)
		}

		log.Printf("Aplicando migração %s...", filename)

		// Executar migração dentro de uma transação
		err = m.ExecuteMigration(ctx, version, string(content))
		if err != nil {
			return fmt.Errorf("erro ao aplicar migração %s: %w", filename, err)
		}

		log.Printf("Migração %s aplicada com sucesso", filename)
	}

	return nil
}

// ExecuteMigration executa uma migração em uma transação
func (m *MigrationManager) ExecuteMigration(ctx context.Context, version, content string) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	// Executar script SQL da migração
	_, err = tx.Exec(ctx, content)
	if err != nil {
		return fmt.Errorf("erro ao executar SQL da migração: %w", err)
	}

	// Marcar migração como aplicada
	query := fmt.Sprintf("INSERT INTO %s.migrations (version) VALUES ($1)", m.schema)
	_, err = tx.Exec(ctx, query, version)
	if err != nil {
		return fmt.Errorf("erro ao marcar migração como aplicada: %w", err)
	}

	// Commit da transação
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("erro ao commit da transação: %w", err)
	}

	return nil
}

// InitializeDatabase inicializa o banco de dados com o esquema inicial
func InitializeDatabase(cfg config.DatabaseConfig) error {
	ctx := context.Background()

	migrationManager, err := NewMigrationManager(cfg)
	if err != nil {
		return err
	}
	defer migrationManager.Close()

	// Executar migrações
	if err := migrationManager.RunMigrations(ctx); err != nil {
		return err
	}

	log.Println("Banco de dados inicializado com sucesso")
	return nil
}
