package database

import (
	"context"
	"fmt"
	"time"

	"GoFacialEmulator/internal/config"
)

// WxsSettings representa as configurações de conexão do WXS
type WxsSettings struct {
	ID        int       `json:"id"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Database  string    `json:"database"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetWxsSettings retorna as configurações do WXS do banco de dados
func (ap *AdaptivePool) GetWxsSettings(ctx context.Context) (*WxsSettings, error) {
	query := `
		SELECT id, host, port, database, username, password, created_at, updated_at
		FROM service.wxs_settings
		ORDER BY id DESC
		LIMIT 1
	`

	var settings WxsSettings
	err := ap.QueryRow(ctx, query).Scan(
		&settings.ID,
		&settings.Host,
		&settings.Port,
		&settings.Database,
		&settings.Username,
		&settings.Password,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get wxs settings: %w", err)
	}

	return &settings, nil
}

// SaveWxsSettings salva ou atualiza as configurações do WXS
func (ap *AdaptivePool) SaveWxsSettings(ctx context.Context, settings *WxsSettings) error {
	// Verificar se já existe um registro
	var count int
	err := ap.QueryRow(ctx, "SELECT COUNT(*) FROM service.wxs_settings").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count wxs settings: %w", err)
	}

	if count > 0 {
		// Atualizar registro existente
		query := `
			UPDATE service.wxs_settings
			SET host = $1, port = $2, database = $3, username = $4, password = $5, updated_at = NOW()
			WHERE id = (SELECT id FROM service.wxs_settings ORDER BY id DESC LIMIT 1)
		`
		_, err = ap.Exec(ctx, query, settings.Host, settings.Port, settings.Database, settings.Username, settings.Password)
	} else {
		// Inserir novo registro
		query := `
			INSERT INTO service.wxs_settings (host, port, database, username, password)
			VALUES ($1, $2, $3, $4, $5)
		`
		_, err = ap.Exec(ctx, query, settings.Host, settings.Port, settings.Database, settings.Username, settings.Password)
	}

	if err != nil {
		return fmt.Errorf("failed to save wxs settings: %w", err)
	}

	return nil
}

// TestWxsConnection testa a conexão com o WXS usando as configurações fornecidas
func TestWxsConnection(settings *WxsSettings) error {
	// Criar configuração temporária
	cfg := config.DatabaseConfig{
		Host:     settings.Host,
		Port:     settings.Port,
		Database: settings.Database,
		Username: settings.Username,
		Password: settings.Password,
		Driver:   "mssql",
	}

	// Tentar criar conexão
	wxsDB, err := NewWxsDB(cfg)
	if err != nil {
		return fmt.Errorf("failed to create connection: %w", err)
	}
	defer wxsDB.Close()

	// Testar ping
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := wxsDB.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Executar query de validação: SELECT COUNT(*) FROM {database}..CfgHWLocalControllers
	query := fmt.Sprintf("SELECT COUNT(*) FROM [%s]..CfgHWLocalControllers", settings.Database)
	var count int
	err = wxsDB.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return fmt.Errorf("validation query failed: %w", err)
	}

	return nil
}
