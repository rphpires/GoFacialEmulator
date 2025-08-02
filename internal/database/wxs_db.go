package database

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"GoFacialEmulator/internal/config"

	_ "github.com/denisenkom/go-mssqldb" // Driver SQL Server
)

// WxsDB gerencia operações no banco WXS (SQL Server)
type WxsDB struct {
	db     *sql.DB
	schema string
}

// NewWxsDB cria uma nova instância do WxsDB para SQL Server
func NewWxsDB(cfg config.DatabaseConfig) (*WxsDB, error) {
	// Connection string para SQL Server
	connString := fmt.Sprintf("server=%s;port=%d;database=%s;user id=%s;password=%s;encrypt=disable;connection timeout=30",
		cfg.Host, cfg.Port, cfg.Database, cfg.Username, cfg.Password)

	db, err := sql.Open("mssql", connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao WXS SQL Server: %w", err)
	}

	// Configurar pool de conexões
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Testar conexão
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("erro ao testar conexão com WXS SQL Server: %w", err)
	}

	return &WxsDB{
		db:     db,
		schema: cfg.Schema,
	}, nil
}

// Close fecha a conexão
func (db *WxsDB) Close() {
	if db.db != nil {
		db.db.Close()
	}
}

// Ping testa a conexão com o banco
func (db *WxsDB) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

// ====================== INTERFACE ADAPTADA PARA SQL SERVER ======================

// Query executa uma query e retorna rows (compatível com sql.DB)
func (db *WxsDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.db.QueryContext(ctx, query, args...)
}

// QueryRow executa uma query que retorna uma única linha
func (db *WxsDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.db.QueryRowContext(ctx, query, args...)
}

// Exec executa uma query sem retorno de dados
func (db *WxsDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.db.ExecContext(ctx, query, args...)
}

// Begin inicia uma transação
func (db *WxsDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return db.db.BeginTx(ctx, opts)
}

// ====================== WXS SPECIFIC OPERATIONS ======================

// GetLocalControllers obtém controladores locais configurados como emuladores
func (db *WxsDB) GetLocalControllers() ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `
		SELECT 
			LocalControllerID, 
			LocalControllerName, 
			IPAddress, 
			BaseCommPort, 
			LocalControllerEnabled,
			LocalControllerType,
			LocalControllerDescription 
		FROM CfgHWLocalControllers
		WHERE LocalControllerDescription LIKE 'emulator%'
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query local controllers: %w", err)
	}
	defer rows.Close()

	var controllers []map[string]interface{}
	for rows.Next() {
		var id, port, controllerType int
		var name, ip, description string
		var enabled bool

		err := rows.Scan(&id, &name, &ip, &port, &enabled, &controllerType, &description)
		if err != nil {
			return nil, fmt.Errorf("failed to scan controller: %w", err)
		}

		// Extrair intervalo de eventos da descrição (formato: emulator_10)
		eventInterval := 0
		if parts := strings.Split(description, "_"); len(parts) == 2 {
			if interval, err := strconv.Atoi(parts[1]); err == nil {
				eventInterval = interval
			}
		}

		// Determinar modelo baseado no tipo
		model := "Unknown"
		switch {
		case contains(HIKVISION_CONTROLLER_TYPES, controllerType):
			model = "Hikvision"
		case contains(DAHUA_CONTROLLER_TYPES, controllerType):
			model = "Dahua"
		}

		controller := map[string]interface{}{
			"LocalControllerID": id,
			"Name":              name,
			"IPAddress":         ip,
			"Port":              port,
			"Enabled":           boolToInt(enabled),
			"Model":             model,
			"Type":              controllerType,
			"EventInterval":     eventInterval,
		}

		controllers = append(controllers, controller)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return controllers, nil
}

// CountCHIDsByLocalController conta CHIDs por controlador local
func (db *WxsDB) CountCHIDsByLocalController() (map[int]map[int][]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query adaptada para SQL Server
	query := `
		SELECT DISTINCT
			sc.ControllerID,
			lc.LocalControllerID,
			lc.BaseCommPort,
			COUNT(DISTINCT ch.CHID) as user_count
		FROM CfgHWLocalControllers lc
		JOIN CfgHWControllers sc ON sc.ControllerID = lc.SiteControllerID
		LEFT JOIN CfgHWReaders rdr ON lc.LocalControllerID = rdr.LocalControllerID
		LEFT JOIN CfgACAccessLevelsContents al_cont ON rdr.ReaderID = al_cont.ReaderID
		LEFT JOIN CHAccessLevels ch ON al_cont.AccessLevelID = ch.AccessLevelID
		WHERE lc.LocalControllerDescription LIKE 'emulator%'
		  AND ch.CHID IN (
			SELECT CHID 
			FROM CHCards
			WHERE IPRdrUserID IS NOT NULL
		  )
		GROUP BY sc.ControllerID, lc.LocalControllerID, lc.BaseCommPort
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count CHIDs: %w", err)
	}
	defer rows.Close()

	result := make(map[int]map[int][]interface{})
	for rows.Next() {
		var siteControllerID, localControllerID, port, userCount int

		err := rows.Scan(&siteControllerID, &localControllerID, &port, &userCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan CHID count: %w", err)
		}

		if result[siteControllerID] == nil {
			result[siteControllerID] = make(map[int][]interface{})
		}

		result[siteControllerID][localControllerID] = []interface{}{port, userCount}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating CHID rows: %w", err)
	}

	return result, nil
}

// CountUsersInSiteControllerDB conta usuários no banco do site controller
func (db *WxsDB) CountUsersInSiteControllerDB() (map[int]map[int]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query para contar usuários por controlador local
	query := `
		SELECT 
			lc.LocalControllerID,
			lc.BaseCommPort,
			COUNT(DISTINCT ch.CHID) as user_count
		FROM CfgHWLocalControllers lc
		LEFT JOIN CfgHWReaders rdr ON lc.LocalControllerID = rdr.LocalControllerID
		LEFT JOIN CfgACAccessLevelsContents al_cont ON rdr.ReaderID = al_cont.ReaderID
		LEFT JOIN CHAccessLevels ch ON al_cont.AccessLevelID = ch.AccessLevelID
		WHERE lc.LocalControllerDescription LIKE 'emulator%'
		GROUP BY lc.LocalControllerID, lc.BaseCommPort
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count users in site controller: %w", err)
	}
	defer rows.Close()

	result := make(map[int]map[int]int)
	for rows.Next() {
		var localControllerID, port, userCount int

		err := rows.Scan(&localControllerID, &port, &userCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user count: %w", err)
		}

		if result[localControllerID] == nil {
			result[localControllerID] = make(map[int]int)
		}

		result[localControllerID][port] = userCount
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user count rows: %w", err)
	}

	return result, nil
}

// ====================== MÉTODOS UTILITÁRIOS ======================

// TestConnection testa se a conexão está funcionando
func (db *WxsDB) TestConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Testar ping
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Testar query simples
	var version string
	err := db.QueryRowContext(ctx, "SELECT @@VERSION").Scan(&version)
	if err != nil {
		return fmt.Errorf("version query failed: %w", err)
	}

	return nil
}

// GetDatabaseInfo retorna informações sobre o banco
func (db *WxsDB) GetDatabaseInfo() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info := make(map[string]interface{})

	// Versão do SQL Server
	var version string
	err := db.QueryRowContext(ctx, "SELECT @@VERSION").Scan(&version)
	if err == nil {
		// Pegar apenas a primeira linha da versão
		lines := strings.Split(version, "\n")
		if len(lines) > 0 {
			info["version"] = strings.TrimSpace(lines[0])
		}
	}

	// Nome do banco
	var dbName string
	err = db.QueryRowContext(ctx, "SELECT DB_NAME()").Scan(&dbName)
	if err == nil {
		info["database"] = dbName
	}

	// Usuário atual
	var currentUser string
	err = db.QueryRowContext(ctx, "SELECT SUSER_NAME()").Scan(&currentUser)
	if err == nil {
		info["user"] = currentUser
	}

	// Contar tabelas principais do WXS
	tables := []string{
		"CfgHWLocalControllers",
		"CfgHWControllers",
		"CfgHWReaders",
		"CHAccessLevels",
		"CHCards",
	}

	tableInfo := make(map[string]int)
	for _, table := range tables {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = '%s'", table)
		err = db.QueryRowContext(ctx, query).Scan(&count)
		if err == nil && count > 0 {
			// Tabela existe, contar registros
			countQuery := fmt.Sprintf("SELECT COUNT(*) FROM [%s]", table)
			var recordCount int
			err = db.QueryRowContext(ctx, countQuery).Scan(&recordCount)
			if err == nil {
				tableInfo[table] = recordCount
			} else {
				tableInfo[table] = -1 // Existe mas não conseguiu contar
			}
		} else {
			tableInfo[table] = 0 // Não existe
		}
	}
	info["tables"] = tableInfo

	return info, nil
}

// ====================== TRANSACTION SUPPORT ======================

// Transaction executa uma função dentro de uma transação
func (db *WxsDB) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback se não foi commitado

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ====================== CONSTANTS AND HELPERS ======================

// Constantes para tipos de controladores (adaptar conforme WXS real)
var (
	HIKVISION_CONTROLLER_TYPES = []int{21101, 21102, 21103} // Adaptar conforme WXS
	DAHUA_CONTROLLER_TYPES     = []int{22111, 22121, 22131} // Adaptar conforme WXS
)

// Funções auxiliares
func contains(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
