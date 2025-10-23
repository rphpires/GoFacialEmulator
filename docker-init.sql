-- Script de inicialização do banco de dados PostgreSQL
-- Executado automaticamente quando o container é criado pela primeira vez

-- Criar banco de dados service_db
CREATE DATABASE service_db;

-- Criar banco de dados emulator_db
CREATE DATABASE emulator_db;

-- Conectar ao service_db e criar schema
\c service_db;
CREATE SCHEMA IF NOT EXISTS service;
GRANT ALL PRIVILEGES ON SCHEMA service TO emulator;

-- Conectar ao emulator_db e criar schema
\c emulator_db;
CREATE SCHEMA IF NOT EXISTS emulator;
GRANT ALL PRIVILEGES ON SCHEMA emulator TO emulator;

-- As tabelas serão criadas automaticamente pela aplicação
-- através do validator.go na inicialização
