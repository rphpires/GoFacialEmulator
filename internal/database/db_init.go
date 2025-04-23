package database

import (
	"context"
	"fmt"
	"log"
	"sync"

	"GoFacialEmulator/internal/config"

	"github.com/jackc/pgx/v4"
)

var (
	serviceDB  *ServiceDB
	emulatorDB *EmulatorDB
	wxsDB      *WxsDB

	serviceOnce  sync.Once
	emulatorOnce sync.Once
	wxsOnce      sync.Once
)

// Initialize inicializa as conexões com o banco de dados
func Initialize(cfg *config.Config) error {
	// Inicializar o banco de dados principal
	if err := InitializeDatabase(cfg.PostgreSQL); err != nil {
		return fmt.Errorf("erro ao inicializar banco de dados: %w", err)
	}

	return nil
}

// GetServiceDB retorna uma instância singleton do ServiceDB
func GetServiceDB(cfg config.DatabaseConfig) (*ServiceDB, error) {
	var err error

	serviceOnce.Do(func() {
		serviceDB, err = NewServiceDB(cfg)
		if err != nil {
			log.Printf("Erro ao criar ServiceDB: %v", err)
			return
		}
	})

	if err != nil {
		return nil, err
	}

	if serviceDB == nil {
		return nil, fmt.Errorf("falha ao inicializar ServiceDB")
	}

	return serviceDB, nil
}

// GetEmulatorDB retorna uma instância singleton do EmulatorDB
func GetEmulatorDB(cfg config.DatabaseConfig) (*EmulatorDB, error) {
	var err error

	emulatorOnce.Do(func() {
		emulatorDB, err = NewEmulatorDB(cfg)
		if err != nil {
			log.Printf("Erro ao criar EmulatorDB: %v", err)
			return
		}
	})

	if err != nil {
		return nil, err
	}

	if emulatorDB == nil {
		return nil, fmt.Errorf("falha ao inicializar EmulatorDB")
	}

	return emulatorDB, nil
}

// GetWxsDB retorna uma instância singleton do WxsDB
func GetWxsDB(cfg config.DatabaseConfig) (*WxsDB, error) {
	var err error

	wxsOnce.Do(func() {
		wxsDB, err = NewWxsDB(cfg)
		if err != nil {
			log.Printf("Erro ao criar WxsDB: %v", err)
			return
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

// CloseAll fecha todas as conexões com o banco de dados
func CloseAll() {
	if serviceDB != nil {
		serviceDB.Close()
	}

	if emulatorDB != nil {
		emulatorDB.Close()
	}

	if wxsDB != nil {
		wxsDB.Close()
	}
}

// WithTransaction executa uma função dentro de uma transação
func WithTransaction(ctx context.Context, db *PostgresDBPool, txFunc func(ctx context.Context) error) error {
	return db.Transaction(ctx, func(tx pgx.Tx) error {
		// Criar um contexto com a transação
		txCtx := context.WithValue(ctx, "tx", tx)
		return txFunc(txCtx)
	})
}
