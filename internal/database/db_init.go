package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"GoFacialEmulator/internal/config"
	"GoFacialEmulator/internal/trace"

	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	serviceDBInterface  DBInterface
	emulatorDBInterface DBInterface
	wxsDB               *WxsDB

	serviceOnce  sync.Once
	emulatorOnce sync.Once
	wxsOnce      sync.Once

	// Flag para usar DualPoolManager (pode ser configurado via variável de ambiente)
	useDualPoolManager = os.Getenv("USE_DUAL_POOL_MANAGER") == "true"
)

func GetServiceDB(cfg config.DatabaseConfig) (DBInterface, error) {
	var err error
	serviceOnce.Do(func() {
		emulatorCount := getEstimatedEmulatorCount(cfg)

		if useDualPoolManager {
			log.Printf("🚀 Creating DualPoolManager for ServiceDB with %d emulators", emulatorCount)
			tracer := trace.NewTracer()
			serviceDBInterface, err = NewDualPoolManager(cfg.PostgresURL(), emulatorCount, tracer)
			if err != nil {
				log.Printf("❌ Failed to create DualPoolManager for ServiceDB: %v", err)
			} else {
				log.Printf("✅ DualPoolManager created successfully for ServiceDB")
			}
		} else {
			log.Printf("Creating AdaptivePool for ServiceDB with %d emulators", emulatorCount)
			var pool *AdaptivePool
			pool, err = NewAdaptivePool(cfg.PostgresURL(), emulatorCount)
			if err != nil {
				log.Printf("Erro ao criar ServiceDB adaptativo: %v", err)
			} else {
				log.Printf("Adaptive pool created successfully for ServiceDB")
			}
			serviceDBInterface = pool
		}
	})
	return serviceDBInterface, err
}

func GetEmulatorDB(cfg config.DatabaseConfig) (DBInterface, error) {
	var err error
	emulatorOnce.Do(func() {
		emulatorCount := getEstimatedEmulatorCount(cfg)

		if useDualPoolManager {
			log.Printf("🚀 Creating DualPoolManager for EmulatorDB with %d emulators", emulatorCount)
			tracer := trace.NewTracer()
			emulatorDBInterface, err = NewDualPoolManager(cfg.PostgresURL(), emulatorCount, tracer)
			if err != nil {
				log.Printf("❌ Failed to create DualPoolManager for EmulatorDB: %v", err)
			} else {
				log.Printf("✅ DualPoolManager created successfully for EmulatorDB")
			}
		} else {
			log.Printf("Creating AdaptivePool for EmulatorDB with %d emulators", emulatorCount)
			var pool *AdaptivePool
			pool, err = NewAdaptivePool(cfg.PostgresURL(), emulatorCount)
			if err != nil {
				log.Printf("Erro ao criar EmulatorDB adaptativo: %v", err)
			}
			emulatorDBInterface = pool
		}
	})
	return emulatorDBInterface, err
}

func GetWxsDB(cfg config.DatabaseConfig) (*WxsDB, error) {
	var err error

	wxsOnce.Do(func() {
		wxsDB, err = NewWxsDB(cfg)
		if err != nil {
			log.Printf("Erro ao criar WxsDB: %v", err)
		}
	})

	if err != nil {
		return nil, err
	}

	if wxsDB == nil {
		return nil, fmt.Errorf("falha ao inicializar WxsDB")
	}

	return wxsDB, nil
}

func CloseAll() {
	// Fechar ServiceDB
	if serviceDBInterface != nil {
		if dpm, ok := serviceDBInterface.(*DualPoolManager); ok {
			dpm.Close()
		} else if ap, ok := serviceDBInterface.(*AdaptivePool); ok {
			ap.Close()
		}
	}

	// Fechar EmulatorDB
	if emulatorDBInterface != nil {
		if dpm, ok := emulatorDBInterface.(*DualPoolManager); ok {
			dpm.Close()
		} else if ap, ok := emulatorDBInterface.(*AdaptivePool); ok {
			ap.Close()
		}
	}

	// Fechar WxsDB
	if wxsDB != nil {
		wxsDB.Close()
	}
}

// GetWxsSettingsFromDB helper function que funciona com qualquer DBInterface
func GetWxsSettingsFromDB(ctx context.Context, db DBInterface) (*WxsSettings, error) {
	query := `
		SELECT id, host, port, database, username, password, created_at, updated_at
		FROM service.wxs_settings
		ORDER BY id DESC
		LIMIT 1
	`

	var settings WxsSettings
	err := db.QueryRow(ctx, query).Scan(
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

// SaveWxsSettingsFromDB helper function que funciona com qualquer DBInterface
func SaveWxsSettingsFromDB(ctx context.Context, db DBInterface, settings *WxsSettings) error {
	// Verificar se já existe um registro
	var count int
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM service.wxs_settings").Scan(&count)
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
		_, err = db.Exec(ctx, query, settings.Host, settings.Port, settings.Database, settings.Username, settings.Password)
	} else {
		// Inserir novo registro. sync_enabled vai explícito como TRUE: gravar
		// uma conexão de W-Access pela primeira vez é, em si, a intenção de
		// sincronizar — sem isso a coluna cairia no DEFAULT TRUE da V002 por
		// acidente, mas a tela de configurações mostraria o toggle
		// desmarcado (settingsPage lê GetSyncEnabled, não a coluna direto),
		// uma divergência entre o banco e o que o operador vê na tela.
		query := `
			INSERT INTO service.wxs_settings (host, port, database, username, password, sync_enabled)
			VALUES ($1, $2, $3, $4, $5, TRUE)
		`
		_, err = db.Exec(ctx, query, settings.Host, settings.Port, settings.Database, settings.Username, settings.Password)
	}

	if err != nil {
		return fmt.Errorf("failed to save wxs settings: %w", err)
	}

	return nil
}

func getEstimatedEmulatorCount(cfg config.DatabaseConfig) int {
	// Tentar contar dispositivos existentes no banco
	// Se falhar, usar estimativa conservadora

	connString := cfg.PostgresURL()
	tempPool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		log.Printf("Não conseguiu conectar para contar dispositivos, usando estimativa padrão: %v", err)
		return 50 // Estimativa conservadora
	}
	defer tempPool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	err = tempPool.QueryRow(ctx, "SELECT COUNT(*) FROM service.devices WHERE enabled = 1").Scan(&count)
	if err != nil {
		log.Printf("Não conseguiu contar dispositivos, usando estimativa padrão: %v", err)
		return 50 // Estimativa conservadora
	}

	if count == 0 {
		count = 20 // Mínimo para desenvolvimento
	}

	log.Printf("Found %d enabled devices in database", count)
	return count
}
