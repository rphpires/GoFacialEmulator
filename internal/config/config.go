package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config representa a configuração da aplicação
type Config struct {
	Server     ServerConfig   `yaml:"server"`
	ServiceDB  DatabaseConfig `yaml:"postgres"`         // Banco principal do serviço
	EmulatorDB DatabaseConfig `yaml:"postgresEmulator"` // Mesmo banco, mas conceitualmente separado
	WxsDB      DatabaseConfig `yaml:"wxsDB"`            // Banco externo WXS
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
	case "postgres":
		return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
			c.Host, c.Port, c.Database, c.Username, c.Password)
	case "postgresEmulator":
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

	// Copiar configuração PostgreSQL para EmulatorDB (mesmo banco)
	config.EmulatorDB = config.ServiceDB

	// Aplicar variáveis de ambiente
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

	// PostgreSQL overrides
	applyDBEnvOverrides(&config.ServiceDB, "PG")
	config.EmulatorDB = config.ServiceDB // Manter sincronizado

	// WXS DB overrides
	applyDBEnvOverrides(&config.WxsDB, "WXS_DB")

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
	if database := os.Getenv(prefix + "_DB"); database != "" {
		db.Database = database
	}
	if username := os.Getenv(prefix + "_USER"); username != "" {
		db.Username = username
	}
	if password := os.Getenv(prefix + "_PASSWORD"); password != "" {
		db.Password = password
	}
	if schema := os.Getenv(prefix + "_SCHEMA"); schema != "" {
		db.Schema = schema
	}
}

// DefaultConfig retorna uma configuração padrão
func DefaultConfig() *Config {
	postgresConfig := DatabaseConfig{
		Driver:   "postgres",
		Host:     "localhost",
		Port:     5432,
		Database: "facial_emulator",
		Username: "emulator",   // Usar o usuário criado
		Password: "#Emul@tor#", // Usar a senha definida
		Schema:   "emulator",
	}

	return &Config{
		Server: ServerConfig{
			Address: ":8080",
			Host:    "localhost",
			Port:    8080,
		},
		ServiceDB:  postgresConfig,
		EmulatorDB: postgresConfig, // Mesmo banco
		WxsDB: DatabaseConfig{
			Driver:   "mssql",
			Host:     "RPH-SRV",
			Port:     1433,
			Database: "W_Access",
			Username: "W-Access",
			Password: "password",
		},
	}
}
