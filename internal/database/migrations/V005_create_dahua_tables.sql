-- Tabela de cartões Dahua (equivalente ao DahuaCard do Python)
CREATE TABLE IF NOT EXISTS emulator.dahua_cards (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    rec_no INTEGER NOT NULL,
    card_name VARCHAR(255) NOT NULL,
    user_id INTEGER NOT NULL,
    card_no VARCHAR(100) NOT NULL,
    valid_date_start TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    valid_date_end TIMESTAMP WITH TIME ZONE DEFAULT NOW() + INTERVAL '10 years',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, rec_no),
    UNIQUE(device_id, card_no),
    UNIQUE(device_id, user_id)
);

-- Tabela de faces Dahua (equivalente ao DahuaFace do Python)
CREATE TABLE IF NOT EXISTS emulator.dahua_faces (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    md5_hash VARCHAR(32),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, user_id)
);

-- Índices para melhor performance Dahua
CREATE INDEX IF NOT EXISTS idx_dahua_cards_device ON emulator.dahua_cards(device_id);
CREATE INDEX IF NOT EXISTS idx_dahua_cards_user ON emulator.dahua_cards(device_id, user_id);
CREATE INDEX IF NOT EXISTS idx_dahua_cards_rec_no ON emulator.dahua_cards(device_id, rec_no);
CREATE INDEX IF NOT EXISTS idx_dahua_faces_device ON emulator.dahua_faces(device_id);
CREATE INDEX IF NOT EXISTS idx_dahua_faces_user ON emulator.dahua_faces(device_id, user_id);