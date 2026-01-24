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

// boolToInt converte bool para int (helper local)
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// WxsDB gerencia operações no banco WXS (SQL Server)
type WxsDB struct {
	db     *sql.DB
	schema string
}

// NewWxsDB cria uma nova instância do WxsDB para SQL Server
func NewWxsDB(cfg config.DatabaseConfig) (*WxsDB, error) {
	// Connection string para SQL Server
	connString := fmt.Sprintf("server=%s;port=%d;database=%s;user id=%s;password=%s;encrypt=disable;connection timeout=10",
		cfg.Host, cfg.Port, cfg.Database, cfg.Username, cfg.Password)

	db, err := sql.Open("mssql", connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao WXS SQL Server: %w", err)
	}

	// Configurar pool de conexões
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// NÃO fazer ping aqui - deixar para o caller decidir
	// Se o servidor não estiver disponível, o ping vai travar a inicialização
	// O ping será feito no main.go após a criação

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

// ====================== WXS SPECIFIC OPERATIONS ======================

// GetLocalControllers obtém controladores locais configurados como emuladores
func (db *WxsDB) GetLocalControllers() ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `
SELECT 
	LocalControllerID, 
	LocalControllerName, 
	lc.IPAddress, 
	lc.BaseCommPort, 
	CASE 
		WHEN ger.ControllerEnabled = 1 THEN LocalControllerEnabled
		ELSE ger.ControllerEnabled
	END AS LocalControllerEnabled,
	LocalControllerType,
	LocalControllerDescription 
FROM CfgHWLocalControllers lc
JOIN CfgHWControllers ger on ger.ControllerID = lc.SiteControllerID
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
