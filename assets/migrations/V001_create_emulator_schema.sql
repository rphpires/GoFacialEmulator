-- ========================================
-- MIGRAÇÃO COMPLETA BASEADA NO PYTHON
-- ========================================

-- V001: Criar schemas
CREATE SCHEMA IF NOT EXISTS emulator;
CREATE SCHEMA IF NOT EXISTS service;

-- ========================================
-- SERVICE SCHEMA (equivalente ao SERVICE_DB_CREATION_STRING do Python)
-- ========================================

-- Tabela Main -> devices (renomeada para consistência)
CREATE TABLE IF NOT EXISTS service.devices (
    local_controller_id INTEGER PRIMARY KEY,
    name TEXT,
    ip_address TEXT,
    port INTEGER,
    model TEXT,
    enabled INTEGER DEFAULT 1,
    type INTEGER,
    status TEXT DEFAULT 'stopped',
    event_interval INTEGER DEFAULT 10,
    total_users INTEGER DEFAULT 0,
    log_enabled INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Tabela UsersCount -> users_comparison (renomeada para consistência)
CREATE TABLE IF NOT EXISTS service.users_comparison (
    site_controller_id INTEGER,
    local_controller_id INTEGER,
    base_comm_port INTEGER,
    wxs_count INTEGER DEFAULT 0,
    site_controller_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(local_controller_id)
);

-- Tabela de configurações do WXS (W-Access)
CREATE TABLE IF NOT EXISTS service.wxs_settings (
    id SERIAL PRIMARY KEY,
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    database TEXT NOT NULL,
    username TEXT NOT NULL,
    password TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ========================================
-- EMULATOR SCHEMA (equivalente ao EMULATOR_DB_CREATION_STRING do Python)
-- ========================================

-- DeviceSettings (configurações globais do dispositivo)
CREATE TABLE IF NOT EXISTS emulator.device_settings (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL DEFAULT 0,  -- Para dispositivos específicos
    cfg_id VARCHAR(255) NOT NULL,
    value TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, cfg_id)
);

-- ========================================
-- TABELAS DAHUA (baseadas no Python)
-- ========================================

-- DahuaCard
CREATE TABLE IF NOT EXISTS emulator.dahua_cards (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL DEFAULT 0,
    rec_no INTEGER NOT NULL,
    card_name TEXT,
    user_id INTEGER NOT NULL,
    card_no TEXT NOT NULL,
    valid_date_start TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    valid_date_end TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '10 years',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, rec_no),
    UNIQUE(device_id, user_id),
    UNIQUE(device_id, card_no)
);

-- DahuaFace
CREATE TABLE IF NOT EXISTS emulator.dahua_faces (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL DEFAULT 0,
    user_id INTEGER NOT NULL,
    md5_hash VARCHAR(32),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, user_id)
);

-- ========================================
-- TABELAS HIKVISION (baseadas no Python)
-- ========================================

-- HikvisionUser
CREATE TABLE IF NOT EXISTS emulator.hikvision_users (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL DEFAULT 0,
    employee_no VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    password VARCHAR(255),
    local_ui_right VARCHAR(10) DEFAULT '0',
    begin_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    end_time TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '10 years',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, employee_no)
);

-- HikvisionCard
CREATE TABLE IF NOT EXISTS emulator.hikvision_cards (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL DEFAULT 0,
    employee_no VARCHAR(50) NOT NULL,
    card_no VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, card_no),
    FOREIGN KEY (device_id, employee_no) REFERENCES emulator.hikvision_users(device_id, employee_no) ON DELETE CASCADE
);

-- HikvisionFace  
CREATE TABLE IF NOT EXISTS emulator.hikvision_faces (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL DEFAULT 0,
    user_id VARCHAR(50) NOT NULL,
    photo_data TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, user_id)
);

-- HikvisionFinger
CREATE TABLE IF NOT EXISTS emulator.hikvision_fingers (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL DEFAULT 0,
    chid VARCHAR(50) NOT NULL,
    data_index INTEGER DEFAULT 1,
    template TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, chid, data_index)
);

-- ========================================
-- ÍNDICES PARA PERFORMANCE
-- ========================================

-- Service
CREATE INDEX IF NOT EXISTS idx_devices_port ON service.devices(port);
CREATE INDEX IF NOT EXISTS idx_devices_status ON service.devices(status);
CREATE INDEX IF NOT EXISTS idx_users_comparison_local_controller ON service.users_comparison(local_controller_id);

-- Emulator
CREATE INDEX IF NOT EXISTS idx_device_settings_device_cfg ON emulator.device_settings(device_id, cfg_id);

-- Dahua
CREATE INDEX IF NOT EXISTS idx_dahua_cards_device ON emulator.dahua_cards(device_id);
CREATE INDEX IF NOT EXISTS idx_dahua_cards_user ON emulator.dahua_cards(device_id, user_id);
CREATE INDEX IF NOT EXISTS idx_dahua_cards_rec_no ON emulator.dahua_cards(device_id, rec_no);
CREATE INDEX IF NOT EXISTS idx_dahua_faces_device ON emulator.dahua_faces(device_id);

-- Hikvision
CREATE INDEX IF NOT EXISTS idx_hikvision_users_device ON emulator.hikvision_users(device_id);
CREATE INDEX IF NOT EXISTS idx_hikvision_cards_device ON emulator.hikvision_cards(device_id);
CREATE INDEX IF NOT EXISTS idx_hikvision_cards_employee ON emulator.hikvision_cards(device_id, employee_no);
CREATE INDEX IF NOT EXISTS idx_hikvision_faces_device ON emulator.hikvision_faces(device_id);
CREATE INDEX IF NOT EXISTS idx_hikvision_fingers_device ON emulator.hikvision_fingers(device_id);

-- ========================================
-- DADOS PADRÃO (equivalente aos INSERTs do Python)
-- ========================================

-- Configurações padrão do dispositivo
INSERT INTO emulator.device_settings (device_id, cfg_id, value) 
VALUES (0, 'LocalAuthentication', '1') 
ON CONFLICT (device_id, cfg_id) DO NOTHING;

INSERT INTO emulator.device_settings (device_id, cfg_id, value) 
VALUES (0, 'RemoteServer', '127.0.0.1') 
ON CONFLICT (device_id, cfg_id) DO NOTHING;

INSERT INTO emulator.device_settings (device_id, cfg_id, value)
VALUES (0, 'RemotePort', '15502')
ON CONFLICT (device_id, cfg_id) DO NOTHING;

-- Configuração do WXS: sem seed de credenciais.
-- O usuário preenche host, banco, usuário e senha na tela /settings.