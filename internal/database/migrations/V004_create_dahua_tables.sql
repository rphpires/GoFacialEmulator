-- Tabela de usuários Dahua
CREATE TABLE IF NOT EXISTS emulator.dahua_users (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    password VARCHAR(255),
    group_id INTEGER DEFAULT 1,
    valid_from TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    valid_to TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '10 years',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, user_id)
);

-- Tabela de cartões Dahua
CREATE TABLE IF NOT EXISTS emulator.dahua_cards (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    rec_no SERIAL,
    card_name VARCHAR(255),
    user_id INTEGER NOT NULL,
    card_no VARCHAR(100) NOT NULL,
    valid_date_start TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    valid_date_end TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '10 years',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, card_no),
    FOREIGN KEY (device_id, user_id) REFERENCES emulator.dahua_users(device_id, user_id) ON DELETE CASCADE
);

-- Tabela de faces Dahua
CREATE TABLE IF NOT EXISTS emulator.dahua_faces (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    md5_hash VARCHAR(32),
    photo_data TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, user_id)
);

-- Índices para Dahua
CREATE INDEX idx_dahua_users_device ON emulator.dahua_users(device_id);
CREATE INDEX idx_dahua_cards_device ON emulator.dahua_cards(device_id);
CREATE INDEX idx_dahua_cards_user ON emulator.dahua_cards(device_id, user_id);
CREATE INDEX idx_dahua_faces_device ON emulator.dahua_faces(device_id);