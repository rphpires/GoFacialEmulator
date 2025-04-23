package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"GoFacialEmulator/internal/config"
	"GoFacialEmulator/internal/trace"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	_ "github.com/lib/pq" // PostgreSQL driver
)

var (
	ErrNoRows = errors.New("no rows in result set")
)

// PostgresDBPool gerencia um pool de conexões para o PostgreSQL
type PostgresDBPool struct {
	pool  *pgxpool.Pool
	trace *trace.Tracer
	mu    sync.RWMutex
}

// NewPostgresDBPool cria um novo pool de conexões com o PostgreSQL
func NewPostgresDBPool(cfg config.DatabaseConfig) (*PostgresDBPool, error) {
	// Configuração da string de conexão PostgreSQL
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	// Configuração do pool de conexões
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao analisar configuração de conexão: %w", err)
	}

	// Configurar limites e parâmetros do pool
	poolConfig.MaxConns = 100                     // Máximo de conexões ativas
	poolConfig.MinConns = 10                      // Mínimo de conexões em espera
	poolConfig.MaxConnLifetime = 1 * time.Hour    // Tempo máximo de vida da conexão
	poolConfig.MaxConnIdleTime = 30 * time.Minute // Tempo máximo de inatividade

	// Criar o pool de conexões
	pool, err := pgxpool.ConnectConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao pool PostgreSQL: %w", err)
	}

	// Verificar a conexão
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("erro ao executar ping no banco: %w", err)
	}

	return &PostgresDBPool{
		pool:  pool,
		trace: trace.NewTracer(),
	}, nil
}

// Close fecha o pool de conexões
func (dp *PostgresDBPool) Close() {
	dp.pool.Close()
}

// GetConn obtém uma conexão do pool
func (dp *PostgresDBPool) GetConn(ctx context.Context) (*pgxpool.Conn, error) {
	return dp.pool.Acquire(ctx)
}

// Query executa uma consulta SQL que retorna linhas
func (dp *PostgresDBPool) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	conn, err := dp.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao adquirir conexão: %w", err)
	}
	defer conn.Release()

	return conn.Query(ctx, query, args...)
}

// QueryRow executa uma consulta SQL que retorna uma única linha
func (dp *PostgresDBPool) QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	conn, err := dp.pool.Acquire(ctx)
	if err != nil {
		return errRow{err: err}
	}
	defer conn.Release()

	return conn.QueryRow(ctx, query, args...)
}

// Exec executa uma consulta SQL que não retorna linhas
func (dp *PostgresDBPool) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	conn, err := dp.pool.Acquire(ctx)
	if err != nil {
		return pgconn.CommandTag{}, fmt.Errorf("erro ao adquirir conexão: %w", err)
	}
	defer conn.Release()

	return conn.Exec(ctx, query, args...)
}

// Transaction executa uma função dentro de uma transação
func (dp *PostgresDBPool) Transaction(ctx context.Context, txFunc func(pgx.Tx) error) error {
	conn, err := dp.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("erro ao adquirir conexão: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}

	if err := txFunc(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("erro na transação: %v, erro no rollback: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("erro ao commit da transação: %w", err)
	}

	return nil
}

// Estrutura auxiliar para manipular erros em QueryRow
type errRow struct {
	err error
}

func (er errRow) Scan(dest ...interface{}) error {
	return er.err
}

// ServiceDB gerencia operações no banco de dados de serviço
type ServiceDB struct {
	*PostgresDBPool
}

// EmulatorDB gerencia operações no banco de dados do emulador
type EmulatorDB struct {
	*PostgresDBPool
}

// NewServiceDB cria um novo manipulador para o banco de dados de serviço
func NewServiceDB(cfg config.DatabaseConfig) (*ServiceDB, error) {
	pool, err := NewPostgresDBPool(cfg)
	if err != nil {
		return nil, err
	}

	return &ServiceDB{pool}, nil
}

// NewEmulatorDB cria um novo manipulador para o banco de dados do emulador
func NewEmulatorDB(cfg config.DatabaseConfig) (*EmulatorDB, error) {
	pool, err := NewPostgresDBPool(cfg)
	if err != nil {
		return nil, err
	}

	return &EmulatorDB{pool}, nil
}

// GetDeviceSettings obtém uma configuração de dispositivo pelo ID
func (db *EmulatorDB) GetDeviceSettings(ctx context.Context, deviceID int, cfgID string) (string, error) {
	var value string
	err := db.QueryRow(ctx,
		"SELECT value FROM emulator.device_settings WHERE device_id = $1 AND cfg_id = $2",
		deviceID, cfgID).Scan(&value)

	if err != nil {
		return "", err
	}
	return value, nil
}

// SetDeviceSettings define uma configuração de dispositivo
func (db *EmulatorDB) SetDeviceSettings(ctx context.Context, deviceID int, cfgID, value string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO emulator.device_settings (device_id, cfg_id, value) 
		 VALUES ($1, $2, $3) 
		 ON CONFLICT (device_id, cfg_id) DO UPDATE SET value = $3`,
		deviceID, cfgID, value)

	return err
}

// GetTotalUsers retorna o número total de usuários para um dispositivo específico
func (db *EmulatorDB) GetTotalUsers(ctx context.Context, deviceType string, deviceID int) (int, error) {
	var count int

	if deviceType == "Dahua" {
		err := db.QueryRow(ctx,
			`SELECT COUNT(*) FROM emulator.dahua_card_devices
			 WHERE device_id = $1`, deviceID).Scan(&count)
		return count, err
	} else if deviceType == "Hikvision" {
		err := db.QueryRow(ctx,
			`SELECT COUNT(*) FROM emulator.hikvision_card_devices
			 WHERE device_id = $1`, deviceID).Scan(&count)
		return count, err
	}

	return 0, fmt.Errorf("tipo de dispositivo não suportado: %s", deviceType)
}

// AddDahuaCard adiciona um novo cartão Dahua
func (db *EmulatorDB) AddDahuaCard(ctx context.Context, deviceID int, cardName string, userID int, cardNo string, validStart, validEnd string) (int64, error) {
	var cardID int64

	err := db.Transaction(ctx, func(tx pgx.Tx) error {
		// Verificar se o cartão já existe
		var exists bool
		err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM emulator.dahua_cards WHERE user_id = $1 OR card_no = $2)`,
			userID, cardNo).Scan(&exists)

		if err != nil {
			return err
		}

		if exists {
			return fmt.Errorf("cartão com UserID=%d ou CardNo=%s já existe", userID, cardNo)
		}

		// Inserir o novo cartão
		err = tx.QueryRow(ctx,
			`INSERT INTO emulator.dahua_cards (card_name, user_id, card_no, valid_date_start, valid_date_end)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			cardName, userID, cardNo, validStart, validEnd).Scan(&cardID)

		if err != nil {
			return err
		}

		// Obter o próximo RecNo para este dispositivo
		var maxRecNo int
		err = tx.QueryRow(ctx,
			`SELECT COALESCE(MAX(rec_no), 0) FROM emulator.dahua_card_devices WHERE device_id = $1`,
			deviceID).Scan(&maxRecNo)

		if err != nil {
			return err
		}

		// Associar o cartão ao dispositivo
		_, err = tx.Exec(ctx,
			`INSERT INTO emulator.dahua_card_devices (card_id, device_id, rec_no)
			 VALUES ($1, $2, $3)`,
			cardID, deviceID, maxRecNo+1)

		return err
	})

	if err != nil {
		return 0, err
	}

	return cardID, nil
}

// RemoveDahuaCard remove um cartão Dahua para um dispositivo específico
func (db *EmulatorDB) RemoveDahuaCard(ctx context.Context, deviceID int, recNo int) error {
	_, err := db.Exec(ctx,
		`DELETE FROM emulator.dahua_card_devices
		 WHERE device_id = $1 AND rec_no = $2`,
		deviceID, recNo)

	return err
}

// AddDahuaFace adiciona uma nova face Dahua
func (db *EmulatorDB) AddDahuaFace(ctx context.Context, userID int, md5 string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO emulator.dahua_faces (user_id, md5)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET md5 = $2`,
		userID, md5)

	return err
}

// RemoveDahuaFace remove uma face Dahua por UserID
func (db *EmulatorDB) RemoveDahuaFace(ctx context.Context, userID int) error {
	_, err := db.Exec(ctx,
		"DELETE FROM emulator.dahua_faces WHERE user_id = $1",
		userID)

	return err
}

// FindDahuaFaces retorna o número total de faces Dahua
func (db *EmulatorDB) FindDahuaFaces(ctx context.Context) (int, error) {
	var count int
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM emulator.dahua_faces").Scan(&count)
	return count, err
}

// GetDahuaFaces retorna uma lista de faces Dahua com paginação
func (db *EmulatorDB) GetDahuaFaces(ctx context.Context, count, offset int) ([]map[string]interface{}, error) {
	rows, err := db.Query(ctx,
		"SELECT user_id, md5 FROM emulator.dahua_faces LIMIT $1 OFFSET $2",
		count, offset)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var faces []map[string]interface{}
	for rows.Next() {
		var userID int
		var md5 string

		if err := rows.Scan(&userID, &md5); err != nil {
			return nil, err
		}

		faces = append(faces, map[string]interface{}{
			"UserID": userID,
			"MD5":    md5,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return faces, nil
}

// FindDahuaCard encontra um cartão Dahua por UserID para um dispositivo específico
func (db *EmulatorDB) FindDahuaCard(ctx context.Context, deviceID int, userID int) (string, []map[string]interface{}, error) {
	rows, err := db.Query(ctx, `
		SELECT dcd.rec_no, dc.card_name, dc.user_id, dc.card_no, dc.valid_date_start, dc.valid_date_end
		FROM emulator.dahua_cards dc
		JOIN emulator.dahua_card_devices dcd ON dc.id = dcd.card_id
		WHERE dcd.device_id = $1 AND dc.user_id = $2
	`, deviceID, userID)

	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var cards []map[string]interface{}
	for rows.Next() {
		var recNo int
		var cardName string
		var userID int
		var cardNo string
		var validStart, validEnd string

		if err := rows.Scan(&recNo, &cardName, &userID, &cardNo, &validStart, &validEnd); err != nil {
			return "", nil, err
		}

		cards = append(cards, map[string]interface{}{
			"RecNo":          recNo,
			"CardName":       cardName,
			"UserID":         userID,
			"CardNo":         cardNo,
			"ValidDateStart": validStart,
			"ValidDateEnd":   validEnd,
		})
	}

	if err := rows.Err(); err != nil {
		return "", nil, err
	}

	if len(cards) == 0 {
		return "found=0", nil, nil
	}

	return "found=1", cards, nil
}

// GetDahuaCards retorna uma lista de cartões Dahua para um dispositivo específico com paginação
func (db *EmulatorDB) GetDahuaCards(ctx context.Context, deviceID int, count, offset int) (string, []map[string]interface{}, error) {
	rows, err := db.Query(ctx, `
		SELECT dcd.rec_no, dc.card_name, dc.user_id, dc.card_no, dc.valid_date_start, dc.valid_date_end
		FROM emulator.dahua_cards dc
		JOIN emulator.dahua_card_devices dcd ON dc.id = dcd.card_id
		WHERE dcd.device_id = $1
		LIMIT $2 OFFSET $3
	`, deviceID, count, offset)

	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var cards []map[string]interface{}
	for rows.Next() {
		var recNo int
		var cardName string
		var userID int
		var cardNo string
		var validStart, validEnd string

		if err := rows.Scan(&recNo, &cardName, &userID, &cardNo, &validStart, &validEnd); err != nil {
			return "", nil, err
		}

		cards = append(cards, map[string]interface{}{
			"RecNo":          recNo,
			"CardName":       cardName,
			"UserID":         userID,
			"CardNo":         cardNo,
			"ValidDateStart": validStart,
			"ValidDateEnd":   validEnd,
		})
	}

	if err := rows.Err(); err != nil {
		return "", nil, err
	}

	if len(cards) == 0 {
		return "found=0", nil, nil
	}

	return fmt.Sprintf("found=%d", len(cards)), cards, nil
}

// Métodos para a API de serviço

// GetDeviceByID retorna informações de um dispositivo pelo ID
func (db *ServiceDB) GetDeviceByID(ctx context.Context, deviceID int) (map[string]interface{}, error) {
	row := db.QueryRow(ctx, `
		SELECT local_controller_id, name, ip_address, port, model, enabled, type, 
		       status, event_interval, total_users, log_enabled
		FROM emulator.devices
		WHERE local_controller_id = $1
	`, deviceID)

	var id int
	var name, ipAddress, model, status string
	var port, deviceType, eventInterval, totalUsers int
	var enabled, logEnabled bool

	err := row.Scan(&id, &name, &ipAddress, &port, &model, &enabled, &deviceType,
		&status, &eventInterval, &totalUsers, &logEnabled)

	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"local_controller_id": id,
		"name":                name,
		"ip_address":          ipAddress,
		"port":                port,
		"model":               model,
		"enabled":             enabled,
		"type":                deviceType,
		"status":              status,
		"event_interval":      eventInterval,
		"total_users":         totalUsers,
		"log_enabled":         logEnabled,
	}, nil
}

// GetAllDevices retorna todos os dispositivos
func (db *ServiceDB) GetAllDevices(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := db.Query(ctx, `
		SELECT local_controller_id, name, ip_address, port, model, enabled, type, 
		       status, event_interval, total_users, log_enabled
		FROM emulator.devices
	`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var id int
		var name, ipAddress, model, status string
		var port, deviceType, eventInterval, totalUsers int
		var enabled, logEnabled bool

		if err := rows.Scan(&id, &name, &ipAddress, &port, &model, &enabled, &deviceType,
			&status, &eventInterval, &totalUsers, &logEnabled); err != nil {
			return nil, err
		}

		devices = append(devices, map[string]interface{}{
			"local_controller_id": id,
			"name":                name,
			"ip_address":          ipAddress,
			"port":                port,
			"model":               model,
			"enabled":             enabled,
			"type":                deviceType,
			"status":              status,
			"event_interval":      eventInterval,
			"total_users":         totalUsers,
			"log_enabled":         logEnabled,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return devices, nil
}

// UpdateDeviceStatus atualiza o status de um dispositivo
func (db *ServiceDB) UpdateDeviceStatus(ctx context.Context, deviceID int, status string) error {
	_, err := db.Exec(ctx, `
		UPDATE emulator.devices 
		SET status = $1, updated_at = NOW() 
		WHERE local_controller_id = $2
	`, status, deviceID)

	return err
}

// UpdateDeviceTotalUsers atualiza o total de usuários de um dispositivo
func (db *ServiceDB) UpdateDeviceTotalUsers(ctx context.Context, deviceID int, totalUsers int) error {
	_, err := db.Exec(ctx, `
		UPDATE emulator.devices 
		SET total_users = $1, updated_at = NOW() 
		WHERE local_controller_id = $2
	`, totalUsers, deviceID)

	return err
}

// SetDeviceLogEnabled define se o log está habilitado para um dispositivo
func (db *ServiceDB) SetDeviceLogEnabled(ctx context.Context, deviceID int, enabled bool) error {
	_, err := db.Exec(ctx, `
		UPDATE emulator.devices 
		SET log_enabled = $1, updated_at = NOW() 
		WHERE local_controller_id = $2
	`, enabled, deviceID)

	return err
}

// UpsertDevice insere ou atualiza um dispositivo
func (db *ServiceDB) UpsertDevice(ctx context.Context, device map[string]interface{}) error {
	_, err := db.Exec(ctx, `
		INSERT INTO emulator.devices (
			local_controller_id, name, ip_address, port, model, enabled, type, 
			status, event_interval, total_users, log_enabled
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (local_controller_id) DO UPDATE SET
			name = $2,
			ip_address = $3,
			port = $4,
			model = $5,
			enabled = $6,
			type = $7,
			status = $8,
			event_interval = $9,
			total_users = $10,
			log_enabled = $11,
			updated_at = NOW()
	`,
		device["local_controller_id"],
		device["name"],
		device["ip_address"],
		device["port"],
		device["model"],
		device["enabled"],
		device["type"],
		device["status"],
		device["event_interval"],
		device["total_users"],
		device["log_enabled"],
	)

	return err
}

// DeleteDevice remove um dispositivo pelo ID
func (db *ServiceDB) DeleteDevice(ctx context.Context, deviceID int) error {
	_, err := db.Exec(ctx, `
		DELETE FROM emulator.devices
		WHERE local_controller_id = $1
	`, deviceID)

	return err
}

// GetUsersComparison retorna a comparação de usuários entre WXS, site controller e emulador
func (db *ServiceDB) GetUsersComparison(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := db.Query(ctx, `
		SELECT uc.site_controller_id, uc.local_controller_id, d.name, uc.base_comm_port,
			   uc.wxs_count, uc.site_controller_count, d.total_users
		FROM emulator.users_comparison uc
		JOIN emulator.devices d ON uc.local_controller_id = d.local_controller_id
	`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comparisons []map[string]interface{}
	for rows.Next() {
		var siteControllerID, localControllerID, baseCommPort, wxsCount, siteControllerCount, emulatorCount int
		var name string

		if err := rows.Scan(&siteControllerID, &localControllerID, &name, &baseCommPort,
			&wxsCount, &siteControllerCount, &emulatorCount); err != nil {
			return nil, err
		}

		comparisons = append(comparisons, map[string]interface{}{
			"site_controller_id":    siteControllerID,
			"local_controller_id":   localControllerID,
			"name":                  name,
			"port":                  baseCommPort,
			"wxs_count":             wxsCount,
			"site_controller_count": siteControllerCount,
			"emulator_count":        emulatorCount,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return comparisons, nil
}

// UpsertUserComparison insere ou atualiza uma comparação de usuários
func (db *ServiceDB) UpsertUserComparison(ctx context.Context, siteControllerID, localControllerID, baseCommPort, wxsCount, siteControllerCount int) error {
	_, err := db.Exec(ctx, `
		INSERT INTO emulator.users_comparison (
			site_controller_id, local_controller_id, base_comm_port, wxs_count, site_controller_count
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (site_controller_id, local_controller_id) DO UPDATE SET
			base_comm_port = $3,
			wxs_count = $4,
			site_controller_count = $5
	`, siteControllerID, localControllerID, baseCommPort, wxsCount, siteControllerCount)

	return err
}
