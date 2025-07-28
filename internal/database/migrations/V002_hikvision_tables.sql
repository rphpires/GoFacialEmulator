-- Tabelas específicas do Hikvision

-- Tabela para usuários Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_users (
    employee_no TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    password TEXT,
    local_ui_right TEXT DEFAULT '0',
    begin_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    end_time TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '1 year',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Tabela para cartões Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_cards (
    employee_no TEXT PRIMARY KEY REFERENCES emulator.hikvision_users(employee_no) ON DELETE CASCADE,
    card_no TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Tabela para armazenar quais dispositivos têm acesso a quais cartões Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_card_devices (
    id SERIAL PRIMARY KEY,
    employee_no TEXT NOT NULL REFERENCES emulator.hikvision_cards(employee_no) ON DELETE CASCADE,
    device_id INTEGER NOT NULL REFERENCES emulator.devices(local_controller_id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE (employee_no, device_id)
);

-- Tabela para faces Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_faces (
    user_id INTEGER PRIMARY KEY,
    photo_data TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Tabela para impressões digitais Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_fingers (
    chid INTEGER NOT NULL,
    data_index INTEGER NOT NULL,
    template TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (chid, data_index)
);

-- Índices para otimização
CREATE INDEX IF NOT EXISTS idx_hikvision_cards_card_no ON emulator.hikvision_cards(card_no);
CREATE INDEX IF NOT EXISTS idx_hikvision_card_devices_device_id ON emulator.hikvision_card_devices(device_id);
CREATE INDEX IF NOT EXISTS idx_hikvision_card_devices_employee_no ON emulator.hikvision_card_devices(employee_no);
CREATE INDEX IF NOT EXISTS idx_hikvision_users_name ON emulator.hikvision_users(name);

-- Permissões
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA emulator TO PUBLIC;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA emulator TO PUBLIC;