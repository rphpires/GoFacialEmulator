package main

import (
	"context"
	"fmt"
	"log"

	"GoFacialEmulator/internal/config"
	"GoFacialEmulator/internal/database"
)

func main() {
	cfg := config.DefaultConfig()

	// Sobrescrever com suas configurações corretas
	cfg.PostgreSQL.Host = "localhost"
	cfg.PostgreSQL.Port = 5432
	cfg.PostgreSQL.Database = "facial_emulator"
	cfg.PostgreSQL.Username = "postgres"
	cfg.PostgreSQL.Password = "testpassword123"
	cfg.PostgreSQL.Schema = "emulator"

	log.Printf("Tentando conectar com: Host=%s, Port=%d, User=%s, Database=%s, Schema=%s",
		cfg.PostgreSQL.Host, cfg.PostgreSQL.Port, cfg.PostgreSQL.Username,
		cfg.PostgreSQL.Database, cfg.PostgreSQL.Schema)

	// Inicializar banco de dados
	log.Println("Inicializando banco de dados...")
	err := database.Initialize(cfg)
	if err != nil {
		log.Fatalf("Erro ao inicializar banco de dados: %v", err)
	}

	// Obter conexão com o banco
	ctx := context.Background()
	serviceDB, err := database.GetServiceDB(cfg.PostgreSQL)
	if err != nil {
		log.Fatalf("Erro ao obter ServiceDB: %v", err)
	}

	// Inserir um dispositivo de teste
	device := map[string]interface{}{
		"local_controller_id": 1,
		"name":                "Emulador de Teste",
		"ip_address":          "127.0.0.1",
		"port":                8080,
		"model":               "Dahua",
		"enabled":             true,
		"type":                22111,
		"status":              "stopped",
		"event_interval":      10,
		"total_users":         0,
		"log_enabled":         false,
	}

	err = serviceDB.UpsertDevice(ctx, device)
	if err != nil {
		log.Fatalf("Erro ao inserir dispositivo: %v", err)
	}
	log.Println("Dispositivo inserido com sucesso!")

	// Ler dispositivos
	devices, err := serviceDB.GetAllDevices(ctx)
	if err != nil {
		log.Fatalf("Erro ao ler dispositivos: %v", err)
	}

	log.Printf("Leitura de %d dispositivos com sucesso!", len(devices))
	for i, dev := range devices {
		fmt.Printf("Dispositivo %d: %s (ID: %d)\n",
			i+1, dev["name"], dev["local_controller_id"])
	}

	log.Println("Teste concluído com sucesso!")
}
