package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"GoFacialEmulator/internal/config"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// WxsDB gerencia operações no banco WXS (equivalente ao sql do Python)
type WxsDB struct {
	pool   *pgxpool.Pool
	schema string
}

// NewWxsDB cria uma nova instância do WxsDB
func NewWxsDB(cfg config.DatabaseConfig) (*WxsDB, error) {
	// Para SQL Server, adaptado para PostgreSQL por enquanto
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao WXS: %w", err)
	}

	return &WxsDB{
		pool:   pool,
		schema: cfg.Schema,
	}, nil
}

// Close fecha a conexão
func (db *WxsDB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Implementar interface DBInterface
func (db *WxsDB) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, query, args...)
}

func (db *WxsDB) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, query, args...)
}

func (db *WxsDB) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, query, args...)
}

func (db *WxsDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return db.pool.Begin(ctx)
}

func (db *WxsDB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// ====================== WXS SPECIFIC OPERATIONS ======================

// GetLocalControllers obtém controladores locais configurados como emuladores - equivalente ao refresh_configured_devices() do Python
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

	rows, err := db.Query(ctx, query)
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

	return controllers, rows.Err()
}

// CountCHIDsByLocalController conta CHIDs por controlador local - equivalente ao wxs_count_chids_by_local_controller() do Python
func (db *WxsDB) CountCHIDsByLocalController() (map[int]map[int][]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query adaptada para a estrutura do WXS
	query := `
		SELECT DISTINCT
			sc.SiteControllerID,
			lc.LocalControllerID,
			lc.BaseCommPort,
			COUNT(DISTINCT ch.CHID) as user_count
		FROM CfgHWLocalControllers lc
		JOIN CfgHWSiteControllers sc ON sc.SiteControllerID = lc.SiteControllerID
		LEFT JOIN CfgHWReaders rdr ON lc.LocalControllerID = rdr.LocalControllerID
		LEFT JOIN CfgACAccessLevelsContents al_cont ON rdr.ReaderID = al_cont.ReaderID
		LEFT JOIN CHAccessLevels ch ON al_cont.AccessLevelID = ch.AccessLevelID
		WHERE lc.LocalControllerDescription LIKE 'emulator%'
		  AND ch.CHID IN (
			SELECT CHID 
			FROM CHCards
			WHERE IPRdrUserID IS NOT NULL
		  )
		GROUP BY sc.SiteControllerID, lc.LocalControllerID, lc.BaseCommPort
	`

	rows, err := db.Query(ctx, query)
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

	return result, rows.Err()
}

// CountUsersInSiteControllerDB conta usuários no banco do site controller - equivalente ao count_users_in_sitecontroller_db() do Python
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

	rows, err := db.Query(ctx, query)
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

	return result, rows.Err()
}

// ====================== CONSTANTS AND HELPERS ======================

// Constantes para tipos de controladores (adaptar conforme necessário baseado no WXS real)
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
