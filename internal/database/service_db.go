package database

import (
	"context"
	"fmt"

	"GoFacialEmulator/internal/config"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// ServiceDB gerencia operações no banco de serviços (equivalente ao service_db do Python)
type ServiceDB struct {
	pool   *pgxpool.Pool
	schema string
}

// NewServiceDB cria uma nova instância do ServiceDB
func NewServiceDB(cfg config.DatabaseConfig) (*ServiceDB, error) {
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao PostgreSQL: %w", err)
	}

	return &ServiceDB{
		pool:   pool,
		schema: cfg.Schema,
	}, nil
}

// Close fecha a conexão
func (db *ServiceDB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Implementar interface DBInterface
func (db *ServiceDB) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, query, args...)
}

func (db *ServiceDB) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, query, args...)
}

func (db *ServiceDB) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, query, args...)
}

func (db *ServiceDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return db.pool.Begin(ctx)
}

func (db *ServiceDB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// ====================== DEVICE MANAGEMENT (equivalente ao Main table do Python) ======================

// GetAllDevices retorna todos os dispositivos - equivalente ao get_current_devices() do Python
func (db *ServiceDB) GetAllDevices(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT local_controller_id, name, ip_address, port, model, status, enabled, 
		       event_interval, total_users, log_enabled, type
		FROM service.devices
		ORDER BY local_controller_id
	`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var id, port, eventInterval, totalUsers, deviceType int
		var name, ipAddress, model, status string
		var enabled, logEnabled bool

		err := rows.Scan(&id, &name, &ipAddress, &port, &model, &status, &enabled,
			&eventInterval, &totalUsers, &logEnabled, &deviceType)
		if err != nil {
			return nil, err
		}

		devices = append(devices, map[string]interface{}{
			"lc_id":       id,
			"name":        name,
			"ip_address":  ipAddress,
			"port":        port,
			"model":       model,
			"status":      status,
			"enabled":     boolToInt(enabled),
			"interval":    eventInterval,
			"total":       totalUsers,
			"log_enabled": boolToInt(logEnabled),
			"type":        deviceType,
		})
	}

	return devices, rows.Err()
}

// UpdateDeviceStatus atualiza o status de um dispositivo
func (db *ServiceDB) UpdateDeviceStatus(ctx context.Context, deviceID int, status string) error {
	_, err := db.Exec(ctx,
		"UPDATE service.devices SET status = $1, updated_at = NOW() WHERE local_controller_id = $2",
		status, deviceID)
	return err
}

// UpdateDeviceTotalUsers atualiza o total de usuários de um dispositivo
func (db *ServiceDB) UpdateDeviceTotalUsers(ctx context.Context, deviceID int, totalUsers int) error {
	_, err := db.Exec(ctx,
		"UPDATE service.devices SET total_users = $1, updated_at = NOW() WHERE local_controller_id = $2",
		totalUsers, deviceID)
	return err
}

// SetLogEnabled atualiza configurações de log por porta - equivalente ao update_log_enabled() do Python
func (db *ServiceDB) SetLogEnabled(ctx context.Context, ports []int, enabled bool) error {
	if len(ports) == 0 {
		return nil
	}

	// Construir query com IN clause
	query := "UPDATE service.devices SET log_enabled = $1, updated_at = NOW() WHERE port = ANY($2)"
	_, err := db.Exec(ctx, query, enabled, ports)
	return err
}

// UpsertDevice insere ou atualiza um dispositivo - equivalente à lógica do refresh_configured_devices() do Python
func (db *ServiceDB) UpsertDevice(ctx context.Context, device map[string]interface{}) error {
	query := `
		INSERT INTO service.devices (
			local_controller_id, name, ip_address, port, model, enabled, type, 
			status, event_interval, total_users, log_enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (local_controller_id) DO UPDATE SET
			name = EXCLUDED.name,
			ip_address = EXCLUDED.ip_address,
			port = EXCLUDED.port,
			model = EXCLUDED.model,
			enabled = EXCLUDED.enabled,
			type = EXCLUDED.type,
			event_interval = EXCLUDED.event_interval,
			updated_at = NOW()
	`

	_, err := db.Exec(ctx, query,
		device["LocalControllerID"],
		device["Name"],
		device["IPAddress"],
		device["Port"],
		device["Model"],
		device["Enabled"],
		device["Type"],
		"stopped", // status inicial
		device["EventInterval"],
		0,     // total_users inicial
		false, // log_enabled inicial
	)

	return err
}

// DeleteDevice remove um dispositivo
func (db *ServiceDB) DeleteDevice(ctx context.Context, deviceID int) error {
	_, err := db.Exec(ctx, "DELETE FROM service.devices WHERE local_controller_id = $1", deviceID)
	return err
}

// ====================== USER COMPARISON (equivalente ao UsersCount table do Python) ======================

// GetUsersComparison retorna dados de comparação - equivalente ao get_comparison_page_content() do Python
func (db *ServiceDB) GetUsersComparison(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			uc.site_controller_id, 
			uc.local_controller_id,
			d.name, 
			uc.base_comm_port,
			uc.wxs_count, 
			uc.site_controller_count, 
			d.total_users 
		FROM service.users_comparison uc
		JOIN service.devices d ON d.local_controller_id = uc.local_controller_id
		ORDER BY uc.local_controller_id
	`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comparisons []map[string]interface{}
	for rows.Next() {
		var siteControllerID, localControllerID, port, wxsCount, siteControllerCount, emulatorCount int
		var name string

		err := rows.Scan(&siteControllerID, &localControllerID, &name, &port,
			&wxsCount, &siteControllerCount, &emulatorCount)
		if err != nil {
			return nil, err
		}

		comparisons = append(comparisons, map[string]interface{}{
			"site_controller_id":    siteControllerID,
			"local_controller_id":   localControllerID,
			"name":                  name,
			"port":                  port,
			"wxs_total":             wxsCount,
			"site_controller_total": siteControllerCount,
			"emulator_total":        emulatorCount,
		})
	}

	return comparisons, rows.Err()
}

// UpsertUserComparison insere ou atualiza comparação de usuários - equivalente ao refresh_users_comparison() do Python
func (db *ServiceDB) UpsertUserComparison(ctx context.Context, siteControllerID, localControllerID, baseCommPort, wxsCount, siteControllerCount int) error {
	// Verificar se existe
	var exists bool
	err := db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM service.users_comparison WHERE local_controller_id = $1)",
		localControllerID).Scan(&exists)
	if err != nil {
		return err
	}

	if exists {
		// Atualizar
		_, err = db.Exec(ctx, `
			UPDATE service.users_comparison 
			SET wxs_count = $1, site_controller_id = $2, base_comm_port = $3, site_controller_count = $4
			WHERE local_controller_id = $5`,
			wxsCount, siteControllerID, baseCommPort, siteControllerCount, localControllerID)
	} else {
		// Inserir
		_, err = db.Exec(ctx, `
			INSERT INTO service.users_comparison 
			(site_controller_id, local_controller_id, base_comm_port, wxs_count, site_controller_count) 
			VALUES ($1, $2, $3, $4, $5)`,
			siteControllerID, localControllerID, baseCommPort, wxsCount, siteControllerCount)
	}

	return err
}

// UpdateSiteControllerCount atualiza contagem do site controller por porta
func (db *ServiceDB) UpdateSiteControllerCount(ctx context.Context, port, count int) error {
	_, err := db.Exec(ctx,
		"UPDATE service.users_comparison SET site_controller_count = $1 WHERE base_comm_port = $2",
		count, port)
	return err
}

// ====================== UTILITY FUNCTIONS ======================

// Transaction executa uma função dentro de uma transação
func (db *ServiceDB) Transaction(ctx context.Context, txFunc func(tx pgx.Tx) error) error {
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

// boolToInt converte bool para int
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
