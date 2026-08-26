-- V002 — emuladores cadastrados fora do W-Access.
--
-- Todo DDL aqui é idempotente de propósito: o validator recria os schemas
-- por DROP CASCADE quando acha estrutura faltando, e nesse caso esta
-- migração é reaplicada do zero.

-- Origem do dispositivo. O DEFAULT é o backfill: toda linha que já existe
-- veio do W-Access, então uma instalação que sincroniza hoje continua se
-- comportando igual depois da atualização.
ALTER TABLE service.devices
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'wxs';

ALTER TABLE service.devices
    DROP CONSTRAINT IF EXISTS devices_source_check;
ALTER TABLE service.devices
    ADD CONSTRAINT devices_source_check CHECK (source IN ('wxs', 'manual'));

CREATE INDEX IF NOT EXISTS idx_devices_source ON service.devices(source);

-- IDs manuais começam acima de qualquer LocalControllerID plausível de um
-- W-Access real. Colisão viola a PK e derruba a transação inteira — falha
-- barulhenta, não corrupção silenciosa.
CREATE SEQUENCE IF NOT EXISTS service.manual_device_id_seq START 900000;

-- O vínculo com o Invenzi vira opção. Sem linha em wxs_settings o sync é
-- considerado desligado, que já é o estado de fato de quem nunca
-- configurou o W-Access.
ALTER TABLE service.wxs_settings
    ADD COLUMN IF NOT EXISTS sync_enabled BOOLEAN NOT NULL DEFAULT TRUE;
