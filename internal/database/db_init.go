package database

import (
	"fmt"
	"log"
	"sync"

	"GoFacialEmulator/internal/config"
)

var (
	// serviceDB  *ServiceDB
	// emulatorDB *EmulatorDB
	serviceDB  *SimpleOptimizedPool
	emulatorDB *SimpleOptimizedPool
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

func GetServiceDB(cfg config.DatabaseConfig) (*SimpleOptimizedPool, error) {
	var err error
	serviceOnce.Do(func() {
		serviceDB, err = NewSimpleOptimizedPool(cfg.PostgresURL())
		if err != nil {
			log.Printf("Erro ao criar ServiceDB: %v", err)
		}
	})
	return serviceDB, err
}

func GetEmulatorDB(cfg config.DatabaseConfig) (*SimpleOptimizedPool, error) {
	var err error
	emulatorOnce.Do(func() {
		emulatorDB, err = NewSimpleOptimizedPool(cfg.PostgresURL())
		if err != nil {
			log.Printf("Erro ao criar EmulatorDB: %v", err)
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
