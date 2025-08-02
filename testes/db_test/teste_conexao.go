package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func main() {
	// Configuração direta da string de conexão
	username := "emulator"
	password := "testpassword123"
	host := "localhost"
	port := 5432
	database := "facial_emulator"

	// Construir a string de conexão diretamente
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		username, password, host, port, database)

	log.Printf("Tentando conectar com string: %s", connString)

	// Configuração do pool de conexões
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		log.Fatalf("Erro ao analisar configuração: %v", err)
	}

	// Configurar limites do pool
	poolConfig.MaxConns = 5
	poolConfig.MinConns = 1
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	// Criar o pool de conexões
	pool, err := pgxpool.ConnectConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalf("Erro ao conectar: %v", err)
	}
	defer pool.Close()

	// Verificar a conexão
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("Erro ao fazer ping: %v", err)
	}

	log.Println("Conexão bem-sucedida!")
}
