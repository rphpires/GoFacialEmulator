-- Schema principal para o emulador
CREATE SCHEMA IF NOT EXISTS emulator;

-- Tabela para configuração do serviço
CREATE TABLE IF NOT EXISTS emulator.service_settings (
    id SERIAL PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL
);

-- Tabela para armazenar informações dos emuladores
CREATE TABLE IF NOT EXISTS emulator.devices (
    local_controller_id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    port INTEGER NOT NULL,
    model TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    type INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'stopped',
    event_interval INTEGER NOT NULL DEFAULT 0,
    total_users INTEGER NOT NULL DEFAULT 0,
    log_enabled BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Tabela de configurações individuais dos dispositivos
CREATE TABLE IF NOT EXISTS emulator.device_settings (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL REFERENCES emulator.devices(local_controller_id) ON DELETE CASCADE,
    cfg_id TEXT NOT NULL,
    value TEXT,
    UNIQUE (device_id, cfg_id)
);

-- Tabela para comparação de usuários (antiga UsersCount)
CREATE TABLE IF NOT EXISTS emulator.users_comparison (
    id SERIAL PRIMARY KEY,
    site_controller_id INTEGER NOT NULL,
    local_controller_id INTEGER NOT NULL REFERENCES emulator.devices(local_controller_id) ON DELETE CASCADE,
    base_comm_port INTEGER,
    wxs_count INTEGER DEFAULT 0,
    site_controller_count INTEGER DEFAULT 0,
    UNIQUE (site_controller_id, local_controller_id)
);

-- Tabelas para cartões Dahua
CREATE TABLE IF NOT EXISTS emulator.dahua_cards (
    id SERIAL PRIMARY KEY,
    card_name TEXT NOT NULL,
    user_id INTEGER NOT NULL UNIQUE,
    card_no TEXT NOT NULL UNIQUE,
    valid_date_start TIMESTAMP,
    valid_date_end TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dahua_cards_user_id ON emulator.dahua_cards(user_id);
CREATE INDEX IF NOT EXISTS idx_dahua_cards_card_no ON emulator.dahua_cards(card_no);

-- Tabela para armazenar quais dispositivos têm acesso a quais cartões
CREATE TABLE IF NOT EXISTS emulator.dahua_card_devices (
    id SERIAL PRIMARY KEY,
    card_id INTEGER NOT NULL REFERENCES emulator.dahua_cards(id) ON DELETE CASCADE,
    device_id INTEGER NOT NULL REFERENCES emulator.devices(local_controller_id) ON DELETE CASCADE,
    rec_no INTEGER NOT NULL,
    UNIQUE (card_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_dahua_card_devices_device_id ON emulator.dahua_card_devices(device_id);

-- Tabela para faces Dahua
CREATE TABLE IF NOT EXISTS emulator.dahua_faces (
    user_id INTEGER PRIMARY KEY,
    md5 TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Tabela para usuários Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_users (
    employee_no TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    password TEXT,
    local_ui_right TEXT,
    begin_time TIMESTAMP,
    end_time TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Tabela para cartões Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_cards (
    employee_no TEXT PRIMARY KEY REFERENCES emulator.hikvision_users(employee_no) ON DELETE CASCADE,
    card_no TEXT NOT NULL UNIQUE
);

CREATE INDEX IF NOT EXISTS idx_hikvision_cards_card_no ON emulator.hikvision_cards(card_no);

-- Tabela para armazenar quais dispositivos têm acesso a quais cartões Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_card_devices (
    id SERIAL PRIMARY KEY,
    employee_no TEXT NOT NULL REFERENCES emulator.hikvision_cards(employee_no) ON DELETE CASCADE,
    device_id INTEGER NOT NULL REFERENCES emulator.devices(local_controller_id) ON DELETE CASCADE,
    UNIQUE (employee_no, device_id)
);

-- Tabela para faces Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_faces (
    user_id INTEGER PRIMARY KEY,
    photo_data TEXT
);

-- Tabela para impressões digitais Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_fingers (
    chid INTEGER NOT NULL,
    data_index INTEGER NOT NULL,
    template TEXT,
    PRIMARY KEY (chid, data_index)
);

-- Inserir configurações padrão
INSERT INTO emulator.service_settings (key, value) VALUES 
('version', '1.0.0'),
('config_update_interval', '60') 
ON CONFLICT (key) DO NOTHING;

-- Funções para triggers

-- Função para atualizar o timestamp de atualização
CREATE OR REPLACE FUNCTION emulator.update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger para atualizar o timestamp dos dispositivos
CREATE TRIGGER update_devices_timestamp
BEFORE UPDATE ON emulator.devices
FOR EACH ROW EXECUTE FUNCTION emulator.update_timestamp();

-- Inicialização de permissões
GRANT USAGE ON SCHEMA emulator TO PUBLIC;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA emulator TO PUBLIC;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA emulator TO PUBLIC;