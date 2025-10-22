package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"GoFacialEmulator/internal/config"

	"github.com/jackc/pgx/v4/pgxpool"
)

// DatabaseValidator valida e recria o banco se necessário
type DatabaseValidator struct {
	pool   *pgxpool.Pool
	config config.DatabaseConfig
}

// NewDatabaseValidator cria um novo validador
func NewDatabaseValidator(cfg config.DatabaseConfig) (*DatabaseValidator, error) {
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar para validação: %w", err)
	}

	return &DatabaseValidator{
		pool:   pool,
		config: cfg,
	}, nil
}

// Close fecha a conexão do validador
func (v *DatabaseValidator) Close() {
	if v.pool != nil {
		v.pool.Close()
	}
}

// ValidateAndRecreateIfNeeded valida o schema e recria se necessário
func (v *DatabaseValidator) ValidateAndRecreateIfNeeded() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Println("🔍 Validando estrutura do banco de dados...")

	// Verificar se a estrutura está correta
	isValid, err := v.validateDatabaseStructure(ctx)
	if err != nil {
		return fmt.Errorf("erro ao validar estrutura: %w", err)
	}

	if isValid {
		log.Println("✅ Estrutura do banco está correta")
		return nil
	}

	log.Println("⚠️  Estrutura do banco não está correta. Recriando...")

	// Recriar banco
	if err := v.recreateDatabase(ctx); err != nil {
		return fmt.Errorf("erro ao recriar banco: %w", err)
	}

	log.Println("✅ Banco recriado com sucesso!")
	return nil
}

// validateDatabaseStructure verifica se todas as tabelas esperadas existem
func (v *DatabaseValidator) validateDatabaseStructure(ctx context.Context) (bool, error) {
	// Tabelas esperadas baseadas no Python
	expectedTables := map[string][]string{
		"service": {
			"devices",          // Main no Python
			"users_comparison", // UsersCount no Python
			"wxs_settings",     // Configurações de conexão WXS
		},
		"emulator": {
			"device_settings",   // DeviceSettings no Python
			"dahua_cards",       // DahuaCard no Python
			"dahua_faces",       // DahuaFace no Python
			"hikvision_users",   // HikvisionUser no Python
			"hikvision_cards",   // HikvisionCard no Python
			"hikvision_faces",   // HikvisionFace no Python
			"hikvision_fingers", // HikvisionFinger no Python
		},
	}

	// Verificar se schemas existem
	for schema := range expectedTables {
		exists, err := v.schemaExists(ctx, schema)
		if err != nil {
			return false, err
		}
		if !exists {
			log.Printf("❌ Schema '%s' não existe", schema)
			return false, nil
		}
	}

	// Verificar se tabelas existem
	for schema, tables := range expectedTables {
		for _, table := range tables {
			exists, err := v.tableExists(ctx, schema, table)
			if err != nil {
				return false, err
			}
			if !exists {
				log.Printf("❌ Tabela '%s.%s' não existe", schema, table)
				return false, nil
			}
		}
	}

	// Verificar colunas críticas
	criticalColumns := map[string]map[string][]string{
		"service": {
			"devices":          {"local_controller_id", "name", "ip_address", "port", "model", "status"},
			"users_comparison": {"site_controller_id", "local_controller_id", "wxs_count"},
		},
		"emulator": {
			"device_settings": {"device_id", "cfg_id", "value"},
			"dahua_cards":     {"device_id", "rec_no", "user_id", "card_no"},
			"hikvision_users": {"device_id", "employee_no", "name"},
		},
	}

	for schema, tables := range criticalColumns {
		for table, columns := range tables {
			for _, column := range columns {
				exists, err := v.columnExists(ctx, schema, table, column)
				if err != nil {
					return false, err
				}
				if !exists {
					log.Printf("❌ Coluna '%s.%s.%s' não existe", schema, table, column)
					return false, nil
				}
			}
		}
	}

	return true, nil
}

// schemaExists verifica se um schema existe
func (v *DatabaseValidator) schemaExists(ctx context.Context, schemaName string) (bool, error) {
	var exists bool
	err := v.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)",
		schemaName).Scan(&exists)
	return exists, err
}

// tableExists verifica se uma tabela existe
func (v *DatabaseValidator) tableExists(ctx context.Context, schemaName, tableName string) (bool, error) {
	var exists bool
	err := v.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2)",
		schemaName, tableName).Scan(&exists)
	return exists, err
}

// columnExists verifica se uma coluna existe
func (v *DatabaseValidator) columnExists(ctx context.Context, schemaName, tableName, columnName string) (bool, error) {
	var exists bool
	err := v.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 AND column_name = $3)",
		schemaName, tableName, columnName).Scan(&exists)
	return exists, err
}

// recreateDatabase apaga e recria toda a estrutura
func (v *DatabaseValidator) recreateDatabase(ctx context.Context) error {
	log.Println("🗑️  Removendo estrutura antiga...")

	// Remover schemas existentes (CASCADE remove todas as tabelas)
	schemas := []string{"emulator", "service"}
	for _, schema := range schemas {
		_, err := v.pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
		if err != nil {
			return fmt.Errorf("erro ao remover schema %s: %w", schema, err)
		}
		log.Printf("   Removido schema: %s", schema)
	}

	log.Println("🔧 Criando nova estrutura...")

	// Executar script de criação
	if err := v.executeCreationScript(ctx); err != nil {
		return fmt.Errorf("erro ao executar script de criação: %w", err)
	}

	return nil
}

// executeCreationScript executa o script de criação do banco
func (v *DatabaseValidator) executeCreationScript(ctx context.Context) error {
	// Ler o arquivo de migração existente
	scriptBytes, err := os.ReadFile("internal/database/migrations/V001_create_emulator_schema.sql")
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo de migração: %w", err)
	}

	script := string(scriptBytes)
	log.Println("   Executando script de migração...")

	// Executar o script completo de uma vez (PostgreSQL suporta múltiplos comandos)
	_, err = v.pool.Exec(ctx, script)
	if err != nil {
		log.Printf("Erro ao executar script completo: %v", err)

		// Se falhar, tentar executar linha por linha (fallback)
		log.Println("   Tentando execução linha por linha...")
		lines := strings.Split(script, "\n")
		var currentCommand strings.Builder

		for i, line := range lines {
			line = strings.TrimSpace(line)

			// Pular linhas vazias e comentários
			if line == "" || strings.HasPrefix(line, "--") {
				continue
			}

			currentCommand.WriteString(line + " ")

			// Se a linha termina com ;, executar o comando
			if strings.HasSuffix(line, ";") {
				cmd := strings.TrimSpace(currentCommand.String())
				if cmd != "" {
					_, err := v.pool.Exec(ctx, cmd)
					if err != nil {
						log.Printf("Erro na linha %d: %s", i+1, cmd)
						return fmt.Errorf("erro ao executar comando na linha %d: %w", i+1, err)
					}
				}
				currentCommand.Reset()
			}
		}

		// Executar último comando se não terminar com ;
		if currentCommand.Len() > 0 {
			cmd := strings.TrimSpace(currentCommand.String())
			if cmd != "" {
				_, err := v.pool.Exec(ctx, cmd)
				if err != nil {
					return fmt.Errorf("erro ao executar último comando: %w", err)
				}
			}
		}
	}

	return nil
}

// ValidateDatabaseOnStartup função principal para ser chamada na inicialização
func ValidateDatabaseOnStartup(cfg config.DatabaseConfig) error {
	validator, err := NewDatabaseValidator(cfg)
	if err != nil {
		return fmt.Errorf("erro ao criar validador: %w", err)
	}
	defer validator.Close()

	return validator.ValidateAndRecreateIfNeeded()
}
