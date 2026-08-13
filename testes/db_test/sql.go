package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
)

func main() {
	// Configurações do SQL Server baseadas no seu config.yaml
	server := "RPH-SRV"
	port := 1433
	database := "W_Access"
	username := "W-Access"
	// Senha real não fica no código: precisa ser exportada antes de rodar
	// este script manual (WXS_PASSWORD=... go run .). Sem a variável, o
	// script roda com senha vazia e falha o login — comportamento esperado.
	password := os.Getenv("WXS_PASSWORD")

	fmt.Println("=== TESTE DE CONEXÃO SQL SERVER ===")
	fmt.Printf("Server: %s:%d\n", server, port)
	fmt.Printf("Database: %s\n", database)
	fmt.Printf("Username: %s\n", username)
	fmt.Println()

	// Teste 1: Diferentes formatos de connection string
	fmt.Println("--- Teste 1: Diferentes Formatos de Connection String ---")
	testConnectionStrings(server, port, database, username, password)
	fmt.Println()

	// Teste 2: Teste de ping básico
	fmt.Println("--- Teste 2: Teste de Conectividade de Rede ---")
	testNetworkConnectivity(server, port)
	fmt.Println()

	// Teste 3: Teste com diferentes configurações
	fmt.Println("--- Teste 3: Diferentes Configurações ---")
	testDifferentConfigs(server, port, database, username, password)
	fmt.Println()

	fmt.Println("=== TESTES CONCLUÍDOS ===")
}

func testConnectionStrings(server string, port int, database, username, password string) {
	// Diferentes formatos de connection string para SQL Server
	connectionStrings := []struct {
		name    string
		connStr string
	}{
		{
			name:    "Formato 1: server=;database=;user id=;password=",
			connStr: fmt.Sprintf("server=%s;port=%d;database=%s;user id=%s;password=%s;encrypt=disable", server, port, database, username, password),
		},
		{
			name:    "Formato 2: Data Source com autenticação SQL",
			connStr: fmt.Sprintf("Data Source=%s,%d;Initial Catalog=%s;User ID=%s;Password=%s;Encrypt=false", server, port, database, username, password),
		},
		{
			name:    "Formato 3: sqlserver:// URL",
			connStr: fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s&encrypt=disable", url.QueryEscape(username), url.QueryEscape(password), server, port, database),
		},
		{
			name:    "Formato 4: Sem especificar porta (usar padrão 1433)",
			connStr: fmt.Sprintf("server=%s;database=%s;user id=%s;password=%s;encrypt=disable", server, database, username, password),
		},
		{
			name:    "Formato 5: Com timeout explícito",
			connStr: fmt.Sprintf("server=%s;port=%d;database=%s;user id=%s;password=%s;encrypt=disable;connection timeout=30", server, port, database, username, password),
		},
	}

	for i, cs := range connectionStrings {
		fmt.Printf("\n%d. %s\n", i+1, cs.name)
		fmt.Printf("   Connection String: %s\n", cs.connStr)

		testSingleConnection(cs.connStr)
	}
}

func testSingleConnection(connString string) {
	// Criar conexão com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("mssql", connString)
	if err != nil {
		fmt.Printf("   ❌ Erro ao abrir conexão: %v\n", err)
		return
	}
	defer db.Close()

	// Configurar timeouts
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Testar ping
	if err := db.PingContext(ctx); err != nil {
		fmt.Printf("   ❌ Erro no ping: %v\n", err)
		return
	}

	// Executar query simples
	var result string
	err = db.QueryRowContext(ctx, "SELECT @@VERSION").Scan(&result)
	if err != nil {
		fmt.Printf("   ❌ Erro ao executar query: %v\n", err)
		return
	}

	fmt.Printf("   ✅ Conexão bem-sucedida!\n")
	fmt.Printf("   📋 Versão do SQL Server: %s\n", truncateString(result, 100))

	// Testar query no banco de dados específico
	var dbName string
	err = db.QueryRowContext(ctx, "SELECT DB_NAME()").Scan(&dbName)
	if err != nil {
		fmt.Printf("   ⚠️  Erro ao obter nome do banco: %v\n", err)
	} else {
		fmt.Printf("   📋 Banco conectado: %s\n", dbName)
	}

	// Testar query com usuário atual
	var currentUser string
	err = db.QueryRowContext(ctx, "SELECT SUSER_NAME()").Scan(&currentUser)
	if err != nil {
		fmt.Printf("   ⚠️  Erro ao obter usuário atual: %v\n", err)
	} else {
		fmt.Printf("   👤 Usuário conectado: %s\n", currentUser)
	}
}

func testNetworkConnectivity(server string, port int) {
	fmt.Printf("Testando conectividade de rede para %s:%d\n", server, port)

	// Teste básico de conectividade TCP
	// Note: Este é um teste simplificado. Em produção, use net.DialTimeout
	connString := fmt.Sprintf("server=%s;port=%d;user id=sa;password=dummy;encrypt=disable;connection timeout=5", server, port)

	db, err := sql.Open("mssql", connString)
	if err != nil {
		fmt.Printf("❌ Erro ao criar conexão de teste: %v\n", err)
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		// Analisar tipo de erro
		errStr := err.Error()
		if contains(errStr, "connection refused") || contains(errStr, "no connection could be made") {
			fmt.Printf("❌ Servidor não está acessível na porta %d\n", port)
			fmt.Printf("   Verifique se:\n")
			fmt.Printf("   - O SQL Server está rodando\n")
			fmt.Printf("   - A porta %d está aberta\n", port)
			fmt.Printf("   - Não há firewall bloqueando\n")
		} else if contains(errStr, "login failed") {
			fmt.Printf("✅ Servidor está acessível, mas credenciais inválidas\n")
			fmt.Printf("   Erro: %v\n", err)
		} else {
			fmt.Printf("❌ Erro de rede: %v\n", err)
		}
	} else {
		fmt.Printf("✅ Conectividade de rede OK\n")
	}
}

func testDifferentConfigs(server string, port int, database, username, password string) {
	configs := []struct {
		name   string
		modify func(string) string
	}{
		{
			name: "Com TrustServerCertificate=true",
			modify: func(base string) string {
				return base + ";TrustServerCertificate=true"
			},
		},
		{
			name: "Com Integrated Security",
			modify: func(base string) string {
				return fmt.Sprintf("server=%s;port=%d;database=%s;Integrated Security=true;encrypt=disable", server, port, database)
			},
		},
		{
			name: "Sem especificar database",
			modify: func(base string) string {
				return fmt.Sprintf("server=%s;port=%d;user id=%s;password=%s;encrypt=disable", server, port, username, password)
			},
		},
		{
			name: "Com ApplicationIntent=ReadOnly",
			modify: func(base string) string {
				return base + ";ApplicationIntent=ReadOnly"
			},
		},
	}

	baseConnString := fmt.Sprintf("server=%s;port=%d;database=%s;user id=%s;password=%s;encrypt=disable", server, port, database, username, password)

	for i, config := range configs {
		fmt.Printf("\n%d. Testando: %s\n", i+1, config.name)
		connString := config.modify(baseConnString)
		fmt.Printf("   Connection String: %s\n", connString)
		testSingleConnection(connString)
	}
}

// Funções auxiliares
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				indexOfSubstring(s, substr) >= 0)))
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Função adicional para testar tabelas específicas do WXS
func testWXSTables(connString string) {
	fmt.Println("\n--- Teste Adicional: Verificar Tabelas WXS ---")

	db, err := sql.Open("mssql", connString)
	if err != nil {
		fmt.Printf("❌ Erro ao abrir conexão: %v\n", err)
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Tabelas que provavelmente existem no WXS baseado no seu código
	tables := []string{
		"CfgHWLocalControllers",
		"CfgHWControllers",
		"CfgHWReaders",
		"CfgACAccessLevelsContents",
		"CHAccessLevels",
		"CHCards",
	}

	fmt.Printf("Verificando tabelas do WXS:\n")
	for _, table := range tables {
		query := fmt.Sprintf("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = '%s'", table)
		var count int
		err := db.QueryRowContext(ctx, query).Scan(&count)

		if err != nil {
			fmt.Printf("  ❌ %s: Erro ao verificar (%v)\n", table, err)
		} else if count > 0 {
			// Tentar contar registros
			countQuery := fmt.Sprintf("SELECT COUNT(*) FROM [%s]", table)
			var recordCount int
			err = db.QueryRowContext(ctx, countQuery).Scan(&recordCount)
			if err != nil {
				fmt.Printf("  ✅ %s: Existe (erro ao contar registros: %v)\n", table, err)
			} else {
				fmt.Printf("  ✅ %s: Existe (%d registros)\n", table, recordCount)
			}
		} else {
			fmt.Printf("  ❌ %s: Não encontrada\n", table)
		}
	}
}
