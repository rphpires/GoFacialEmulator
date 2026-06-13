package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config representa a configuração da aplicação
type Config struct {
	AppVersion string         `yaml:"app_version"`
	Server     ServerConfig   `yaml:"server"`
	ServiceDB  DatabaseConfig `yaml:"service_db"`  // Banco principal do serviço
	EmulatorDB DatabaseConfig `yaml:"emulator_db"` // Mesmo banco, mas conceitualmente separado
	WxsDB      DatabaseConfig `yaml:"wxsDB"`       // Banco externo WXS
}

// ServerConfig contém as configurações do servidor HTTP
type ServerConfig struct {
	Address string `yaml:"address" env:"SERVER_ADDR"`
	Host    string `yaml:"host" env:"HOST_ADDR"`
	Port    int    `yaml:"port" env:"PORT_ADDR"`
}

// DatabaseConfig contém as configurações de conexão com o banco de dados
type DatabaseConfig struct {
	Driver             string `yaml:"driver"`
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	Database           string `yaml:"database"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	Schema             string `yaml:"schema,omitempty"`
	MaxConnections     int    `yaml:"max_connections,omitempty"`
	MinConnections     int    `yaml:"min_connections,omitempty"`
	ConnectionLifetime string `yaml:"connection_lifetime,omitempty"`
}

// DSN retorna a string de conexão para o banco de dados
func (c *DatabaseConfig) DSN() string {
	switch c.Driver {
	case "service_db":
		return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
			c.Host, c.Port, c.Database, c.Username, c.Password)
	case "emulator_db":
		return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
			c.Host, c.Port, c.Database, c.Username, c.Password)
	case "mssql":
		return fmt.Sprintf("server=%s;port=%d;database=%s;user id=%s;password=%s;encrypt=disable",
			c.Host, c.Port, c.Database, c.Username, c.Password)
	default:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
			c.Username, c.Password, c.Host, c.Port, c.Database)
	}
}

// loadEnvFile carrega variáveis do arquivo .env
func loadEnvFile() error {
	envFiles := []string{".env.local", ".env", ".env.development"}

	for _, envFile := range envFiles {
		if _, err := os.Stat(envFile); err == nil {
			return loadSingleEnvFile(envFile)
		}
	}

	// Se nenhum arquivo .env for encontrado, não é erro
	return nil
}

// loadSingleEnvFile carrega um arquivo .env específico
func loadSingleEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return nil // Não é erro se arquivo não existir
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Pular linhas vazias e comentários
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Processar linha no formato KEY=VALUE
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Remover aspas se existirem
			value = strings.Trim(value, `"'`)

			// Só definir se não existir como variável de ambiente
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}

	return scanner.Err()
}

// Load carrega a configuração a partir de um arquivo YAML
func Load(path string) (*Config, error) {
	// Primeiro, carregar variáveis do arquivo .env
	if err := loadEnvFile(); err != nil {
		return nil, fmt.Errorf("erro ao carregar arquivo .env: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler arquivo de configuração: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("erro ao fazer parse do arquivo de configuração: %w", err)
	}

	// Copiar configuração PostgreSQL para EmulatorDB (mesmo banco)
	config.EmulatorDB = config.ServiceDB

	// Aplicar variáveis de ambiente (agora incluindo as do .env)
	if err := applyEnvOverrides(config); err != nil {
		return nil, fmt.Errorf("erro ao aplicar overrides de ambiente: %w", err)
	}

	return config, nil
}

// applyEnvOverrides aplica overrides das variáveis de ambiente
func applyEnvOverrides(config *Config) error {
	// Server overrides
	if addr := os.Getenv("SERVER_ADDR"); addr != "" {
		config.Server.Address = addr
	}
	if host := os.Getenv("SERVER_HOST"); host != "" {
		config.Server.Host = host
	}
	if portStr := os.Getenv("SERVER_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			config.Server.Port = port
		}
	}

	// PostgreSQL overrides (service_db e emulator_db)
	applyDBEnvOverrides(&config.ServiceDB, "PG")

	// Também aplicar overrides diretos para PostgreSQL
	applyDBEnvOverrides(&config.ServiceDB, "POSTGRES")
	applyDBEnvOverrides(&config.ServiceDB, "DB")

	// Manter EmulatorDB sincronizado
	config.EmulatorDB = config.ServiceDB

	// WXS DB overrides
	applyDBEnvOverrides(&config.WxsDB, "WXS")
	applyDBEnvOverrides(&config.WxsDB, "WXS_DB")
	applyDBEnvOverrides(&config.WxsDB, "MSSQL")

	return nil
}

func (c *DatabaseConfig) PostgresURL() string {
	if c.Driver != "postgres" {
		return ""
	}

	url := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.Username, c.Password, c.Host, c.Port, c.Database)

	// Adicionar schema se especificado
	if c.Schema != "" {
		url += fmt.Sprintf("&search_path=%s", c.Schema)
	}

	return url
}

// applyDBEnvOverrides aplica overrides de ambiente para configuração de banco
func applyDBEnvOverrides(db *DatabaseConfig, prefix string) {
	if host := os.Getenv(prefix + "_HOST"); host != "" {
		db.Host = host
	}
	if portStr := os.Getenv(prefix + "_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			db.Port = port
		}
	}
	if database := os.Getenv(prefix + "_DATABASE"); database != "" {
		db.Database = database
	}
	if database := os.Getenv(prefix + "_DB"); database != "" {
		db.Database = database
	}
	if username := os.Getenv(prefix + "_USER"); username != "" {
		db.Username = username
	}
	if username := os.Getenv(prefix + "_USERNAME"); username != "" {
		db.Username = username
	}
	if password := os.Getenv(prefix + "_PASSWORD"); password != "" {
		db.Password = password
	}
	if driver := os.Getenv(prefix + "_DRIVER"); driver != "" {
		db.Driver = driver
	}
	if schema := os.Getenv(prefix + "_SCHEMA"); schema != "" {
		db.Schema = schema
	}
	if maxConnStr := os.Getenv(prefix + "_MAX_CONNECTIONS"); maxConnStr != "" {
		if maxConn, err := strconv.Atoi(maxConnStr); err == nil {
			db.MaxConnections = maxConn
		}
	}
}

// GetEffectiveConfig retorna a configuração final com todos os overrides aplicados
func (c *Config) GetEffectiveConfig() map[string]interface{} {
	return map[string]interface{}{
		"app_version": c.AppVersion,
		"server": map[string]interface{}{
			"host": c.Server.Host,
			"port": c.Server.Port,
		},
		"service_db": map[string]interface{}{
			"driver":   c.ServiceDB.Driver,
			"host":     c.ServiceDB.Host,
			"port":     c.ServiceDB.Port,
			"database": c.ServiceDB.Database,
			"username": c.ServiceDB.Username,
			"password": "***", // Não mostrar senha completa
		},
		"wxs_db": map[string]interface{}{
			"driver":   c.WxsDB.Driver,
			"host":     c.WxsDB.Host,
			"port":     c.WxsDB.Port,
			"database": c.WxsDB.Database,
			"username": c.WxsDB.Username,
			"password": "***", // Não mostrar senha completa
		},
	}
}
