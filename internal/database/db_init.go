package database

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"GoFacialEmulator/internal/config"

	"github.com/jackc/pgx/v4/pgxpool"
)

var (
	serviceDB  *AdaptivePool
	emulatorDB *AdaptivePool
	wxsDB      *WxsDB

	serviceOnce  sync.Once
	emulatorOnce sync.Once
	wxsOnce      sync.Once
)

func Initialize(cfg *config.Config) error {
	log.Println("Inicializando conexões com banco de dados...")

	// A validação já foi feita no main.go com ValidateDatabaseOnStartup()
	// Aqui apenas confirmamos que as conexões funcionam

	return nil
}

func GetServiceDB(cfg config.DatabaseConfig) (*AdaptivePool, error) {
	var err error
	serviceOnce.Do(func() {
		emulatorCount := getEstimatedEmulatorCount(cfg)

		log.Printf("Creating adaptive pool for %d estimated emulators", emulatorCount)
		serviceDB, err = NewAdaptivePool(cfg.PostgresURL(), emulatorCount)
		if err != nil {
			log.Printf("Erro ao criar ServiceDB adaptativo: %v", err)
		} else {
			log.Printf("Adaptive pool created successfully")
		}
	})
	return serviceDB, err
}

func GetEmulatorDB(cfg config.DatabaseConfig) (*AdaptivePool, error) {
	var err error
	emulatorOnce.Do(func() {
		// Para EmulatorDB, usar estimativa similar
		emulatorCount := getEstimatedEmulatorCount(cfg)

		log.Printf("Creating adaptive emulator pool for %d estimated emulators", emulatorCount)
		emulatorDB, err = NewAdaptivePool(cfg.PostgresURL(), emulatorCount)
		if err != nil {
			log.Printf("Erro ao criar EmulatorDB adaptativo: %v", err)
		}
	})
	return emulatorDB, err
}

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
	err = tempPool.QueryRow(ctx, "SELECT COUNT(*) FROM service.devices WHERE enabled = true").Scan(&count)
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
