package main

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

func main() {
	// Obter usuário atual do sistema
	currentUser, _ := user.Current()
	fmt.Printf("Usuário atual do sistema: %s\n", currentUser.Username)
	fmt.Printf("Home directory: %s\n", currentUser.HomeDir)

	// Verificar variáveis de ambiente PostgreSQL
	fmt.Println("\n=== Variáveis de Ambiente PostgreSQL ===")
	checkEnvVar("PGHOST")
	checkEnvVar("PGPORT")
	checkEnvVar("PGDATABASE")
	checkEnvVar("PGUSER")
	checkEnvVar("PGPASSWORD")
	checkEnvVar("DATABASE_URL")

	// Configurações do seu arquivo YAML
	host := "localhost"
	port := 5432
	database := "facial_emulator"
	username := "emulator"
	password := "#Emul@tor#"
	schema := "emulator"

	fmt.Println("\n=== Teste de Conexão ===")
	fmt.Printf("Tentando conectar com:\n")
	fmt.Printf("  Host: %s\n", host)
	fmt.Printf("  Port: %d\n", port)
	fmt.Printf("  Database: %s\n", database)
	fmt.Printf("  Username: %s\n", username)
	fmt.Printf("  Schema: %s\n", schema)

	// Teste 1: Conexão básica com pgx
	fmt.Println("\n--- Teste 1: Conexão básica com pgx ---")
	testBasicConnection(host, port, database, username, password)

	// Teste 2: Conexão com pool
	fmt.Println("\n--- Teste 2: Conexão com pool ---")
	testPoolConnection(host, port, database, username, password, schema)

	// Teste 3: Diferentes formatos de connection string
	fmt.Println("\n--- Teste 3: Diferentes formatos de connection string ---")
	testConnectionStrings(host, port, database, username, password, schema)

	// Teste 4: Verificar se o banco/usuário existe
	fmt.Println("\n--- Teste 4: Verificar banco/usuário ---")
	testDatabaseExists(host, port, username, password)
}

func checkEnvVar(name string) {
	value := os.Getenv(name)
	if value != "" {
		fmt.Printf("%s = %s\n", name, value)
	} else {
		fmt.Printf("%s = (não definida)\n", name)
	}
}

func testBasicConnection(host string, port int, database, username, password string) {
	connString := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		host, port, database, username, password)

	fmt.Printf("Connection string: %s\n", connString)

	conn, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		fmt.Printf("❌ Erro na conexão básica: %v\n", err)
		return
	}
	defer conn.Close(context.Background())

	// Testar query simples
	var result string
	err = conn.QueryRow(context.Background(), "SELECT current_user").Scan(&result)
	if err != nil {
		fmt.Printf("❌ Erro ao executar query: %v\n", err)
		return
	}

	fmt.Printf("✅ Conexão básica bem-sucedida! Usuário conectado: %s\n", result)
}

func testPoolConnection(host string, port int, database, username, password, schema string) {
	connString := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		username, password, host, port, database)

	if schema != "" {
		connString += fmt.Sprintf("&search_path=%s", schema)
	}

	fmt.Printf("Pool connection string: %s\n", connString)

	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		fmt.Printf("❌ Erro ao fazer parse da config: %v\n", err)
		return
	}

	pool, err := pgxpool.ConnectConfig(context.Background(), config)
	if err != nil {
		fmt.Printf("❌ Erro ao conectar com pool: %v\n", err)
		return
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		fmt.Printf("❌ Erro no ping: %v\n", err)
		return
	}

	// Testar query
	var user, currentSchema string
	err = pool.QueryRow(ctx, "SELECT current_user, current_schema()").Scan(&user, &currentSchema)
	if err != nil {
		fmt.Printf("❌ Erro ao executar query: %v\n", err)
		return
	}

	fmt.Printf("✅ Pool connection bem-sucedida! Usuário: %s, Schema: %s\n", user, currentSchema)
}

func testConnectionStrings(host string, port int, database, username, password, schema string) {
	connectionStrings := []string{
		// Formato 1: key=value
		fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
			host, port, database, username, password),

		// Formato 2: URL
		fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
			username, password, host, port, database),

		// Formato 3: URL com schema
		fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable&search_path=%s",
			username, password, host, port, database, schema),

		// Formato 4: key=value com schema
		fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable search_path=%s",
			host, port, database, username, password, schema),
	}

	for i, connStr := range connectionStrings {
		fmt.Printf("\nTestando formato %d: %s\n", i+1, connStr)

		conn, err := pgx.Connect(context.Background(), connStr)
		if err != nil {
			fmt.Printf("❌ Falhou: %v\n", err)
			continue
		}

		var user, currentSchema string
		err = conn.QueryRow(context.Background(), "SELECT current_user, current_schema()").Scan(&user, &currentSchema)
		conn.Close(context.Background())

		if err != nil {
			fmt.Printf("❌ Erro na query: %v\n", err)
		} else {
			fmt.Printf("✅ Sucesso! Usuário: %s, Schema: %s\n", user, currentSchema)
		}
	}
}

func testDatabaseExists(host string, port int, username, password string) {
	// Conectar ao banco postgres (padrão) para verificar se o banco existe
	connString := fmt.Sprintf("host=%s port=%d dbname=postgres user=%s password=%s sslmode=disable",
		host, port, username, password)

	fmt.Printf("Conectando ao banco 'postgres' para verificar...\n")

	conn, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		fmt.Printf("❌ Não foi possível conectar ao banco postgres: %v\n", err)

		// Tentar com usuário postgres
		connString2 := fmt.Sprintf("host=%s port=%d dbname=postgres user=postgres password=%s sslmode=disable",
			host, port, password)
		fmt.Printf("Tentando com usuário 'postgres'...\n")

		conn2, err2 := pgx.Connect(context.Background(), connString2)
		if err2 != nil {
			fmt.Printf("❌ Também falhou com usuário postgres: %v\n", err2)
			return
		}
		conn = conn2
	}
	defer conn.Close(context.Background())

	// Verificar se o banco facial_emulator existe
	var exists bool
	err = conn.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)",
		"facial_emulator").Scan(&exists)

	if err != nil {
		fmt.Printf("❌ Erro ao verificar se banco existe: %v\n", err)
		return
	}

	if exists {
		fmt.Printf("✅ Banco 'facial_emulator' existe\n")
	} else {
		fmt.Printf("❌ Banco 'facial_emulator' NÃO existe\n")
	}

	// Verificar se usuário emulator existe
	var userExists bool
	err = conn.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM pg_user WHERE usename = $1)",
		"emulator").Scan(&userExists)

	if err != nil {
		fmt.Printf("❌ Erro ao verificar se usuário existe: %v\n", err)
		return
	}

	if userExists {
		fmt.Printf("✅ Usuário 'emulator' existe\n")
	} else {
		fmt.Printf("❌ Usuário 'emulator' NÃO existe\n")
	}
}
