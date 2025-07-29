CREATE TABLE IF NOT EXISTS emulator.device_settings (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL,
    cfg_id VARCHAR(255) NOT NULL,
    value TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(device_id, cfg_id)
);

CREATE INDEX idx_device_settings_device_cfg ON emulator.device_settings(device_id, cfg_id);
