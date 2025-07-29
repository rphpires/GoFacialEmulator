-- Tabela de usuários Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_users (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
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

-- Tabela de cartões Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_cards (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    employee_no VARCHAR(50) NOT NULL,
    card_no VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, card_no),
    FOREIGN KEY (device_id, employee_no) REFERENCES emulator.hikvision_users(device_id, employee_no) ON DELETE CASCADE
);

-- Tabela de faces Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_faces (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    user_id VARCHAR(50) NOT NULL,
    photo_data TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, user_id)
);

-- Tabela de impressões digitais Hikvision
CREATE TABLE IF NOT EXISTS emulator.hikvision_fingers (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    chid VARCHAR(50) NOT NULL,
    data_index INTEGER DEFAULT 1,
    template TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, chid, data_index)
);

-- Índices para melhor performance
CREATE INDEX idx_hikvision_users_device ON emulator.hikvision_users(device_id);
CREATE INDEX idx_hikvision_cards_device ON emulator.hikvision_cards(device_id);
CREATE INDEX idx_hikvision_cards_employee ON emulator.hikvision_cards(device_id, employee_no);
CREATE INDEX idx_hikvision_faces_device ON emulator.hikvision_faces(device_id);
CREATE INDEX idx_hikvision_fingers_device ON emulator.hikvision_fingers(device_id);