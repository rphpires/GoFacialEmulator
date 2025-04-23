package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config representa a configuração da aplicação
type Config struct {
	Server     ServerConfig   `yaml:"server"`
	PostgreSQL DatabaseConfig `yaml:"postgres"`
	WxsDB      DatabaseConfig `yaml:"wxsDB"`
}

// ServerConfig contém as configurações do servidor HTTP
type ServerConfig struct {
	Address string `yaml:"address"`
}

// DatabaseConfig contém as configurações de conexão com o banco de dados
type DatabaseConfig struct {
	Driver   string `yaml:"driver"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Schema   string `yaml:"schema"`
}

// DSN retorna a string de conexão para o banco de dados
func (c *DatabaseConfig) DSN() string {
	switch c.Driver {
	case "postgres":
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

// Load carrega a configuração a partir de um arquivo YAML
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler arquivo de configuração: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("erro ao fazer parse do arquivo de configuração: %w", err)
	}

	// Carregar de variáveis de ambiente se fornecidas
	if os.Getenv("SERVER_ADDR") != "" {
		config.Server.Address = os.Getenv("SERVER_ADDR")
	}

	// PostgreSQL config from env
	if os.Getenv("PG_HOST") != "" {
		config.PostgreSQL.Host = os.Getenv("PG_HOST")
	}
	if os.Getenv("PG_PORT") != "" {
		var port int
		if _, err := fmt.Sscanf(os.Getenv("PG_PORT"), "%d", &port); err == nil {
			config.PostgreSQL.Port = port
		}
	}
	if os.Getenv("PG_DB") != "" {
		config.PostgreSQL.Database = os.Getenv("PG_DB")
	}
	if os.Getenv("PG_USER") != "" {
		config.PostgreSQL.Username = os.Getenv("PG_USER")
	}
	if os.Getenv("PG_PASSWORD") != "" {
		config.PostgreSQL.Password = os.Getenv("PG_PASSWORD")
	}
	if os.Getenv("PG_SCHEMA") != "" {
		config.PostgreSQL.Schema = os.Getenv("PG_SCHEMA")
	}

	// WXS DB config from env
	if os.Getenv("WXS_DB_HOST") != "" {
		config.WxsDB.Host = os.Getenv("WXS_DB_HOST")
	}
	if os.Getenv("WXS_DB_PORT") != "" {
		var port int
		if _, err := fmt.Sscanf(os.Getenv("WXS_DB_PORT"), "%d", &port); err == nil {
			config.WxsDB.Port = port
		}
	}
	if os.Getenv("WXS_DB_NAME") != "" {
		config.WxsDB.Database = os.Getenv("WXS_DB_NAME")
	}
	if os.Getenv("WXS_DB_USER") != "" {
		config.WxsDB.Username = os.Getenv("WXS_DB_USER")
	}
	if os.Getenv("WXS_DB_PASSWORD") != "" {
		config.WxsDB.Password = os.Getenv("WXS_DB_PASSWORD")
	}

	return config, nil
}

// DefaultConfig retorna uma configuração padrão
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address: ":8080",
		},
		PostgreSQL: DatabaseConfig{
			Driver:   "postgres",
			Host:     "localhost",
			Port:     5432,
			Database: "facial_emulator",
			Username: "postgres",
			Password: "testpassword123",
			Schema:   "emulator",
		},
		WxsDB: DatabaseConfig{
			Driver:   "mssql",
			Host:     "127.0.0.1",
			Port:     1433,
			Database: "W_Access",
			Username: "W-Access",
			Password: "password",
		},
	}
}
