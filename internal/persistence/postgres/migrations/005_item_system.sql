-- 1. Clean up deprecated JSONB storage columns & tables
DROP TABLE IF EXISTS player_station_vaults CASCADE;
DROP TABLE IF EXISTS corporation_station_vaults CASCADE;
ALTER TABLE stations DROP COLUMN IF EXISTS cargo;
ALTER TABLE characters DROP COLUMN IF EXISTS cargo;

-- 2. Create Item Definitions (Templates) table
CREATE TABLE IF NOT EXISTS item_definitions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL,      -- 'resource', 'material', 'module', 'ship', 'blueprint'
    stackable BOOLEAN NOT NULL DEFAULT true,
    volume REAL NOT NULL DEFAULT 0.0,
    meta_data JSONB DEFAULT '{}'::jsonb
);

-- 3. Create Item Instances table
CREATE TABLE IF NOT EXISTS item_instances (
    id BIGSERIAL PRIMARY KEY,
    definition_id INT NOT NULL REFERENCES item_definitions(id) ON DELETE RESTRICT,
    quantity INT NOT NULL DEFAULT 1,
    location_type VARCHAR(50) NOT NULL, -- 'SHIP_CARGO', 'STATION_PERSONAL_VAULT', 'STATION_CORP_VAULT', 'STATION_MARKET', 'LOOT_CONTAINER'
    location_id BIGINT NOT NULL,        -- player account ID, station ID, etc.
    owner_id BIGINT,                    -- player account ID (nullable, e.g. for personal safe or cargo)
    state VARCHAR(50) NOT NULL DEFAULT 'normal', -- 'normal', 'locked', 'destroyed'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexing for fast lookups
CREATE INDEX IF NOT EXISTS idx_item_instances_location ON item_instances(location_type, location_id);
CREATE INDEX IF NOT EXISTS idx_item_instances_owner ON item_instances(owner_id);

-- 4. Seed Item Definitions
INSERT INTO item_definitions (id, name, category, stackable, volume, meta_data) VALUES
(1, 'Iron', 'resource', true, 0.1, '{}'::jsonb),
(2, 'Titanium', 'resource', true, 0.2, '{}'::jsonb),
(3, 'Crystal', 'resource', true, 0.5, '{}'::jsonb),
(4, 'RareGas', 'resource', true, 0.3, '{}'::jsonb),
(5, 'IronPlates', 'material', true, 0.15, '{}'::jsonb),
(6, 'TitaniumPlates', 'material', true, 0.25, '{}'::jsonb),
(7, 'Laser Cannon', 'module', false, 2.0, '{}'::jsonb),
(8, 'Mining Laser', 'module', false, 3.0, '{}'::jsonb),
(9, 'Fighter Ship', 'ship', false, 50.0, '{}'::jsonb),
(10, 'Miner Ship', 'ship', false, 80.0, '{}'::jsonb),
(11, 'SiliconWafers', 'material', true, 0.15, '{}'::jsonb),
(12, 'FuelCells', 'material', true, 0.2, '{}'::jsonb),
(13, 'Microchips', 'material', true, 0.05, '{}'::jsonb),
(14, 'EnergyCoils', 'material', true, 0.1, '{}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    category = EXCLUDED.category,
    stackable = EXCLUDED.stackable,
    volume = EXCLUDED.volume,
    meta_data = EXCLUDED.meta_data;

-- Seed initial market inventory for stations in item_instances
INSERT INTO item_instances (definition_id, quantity, location_type, location_id, owner_id) VALUES
-- Station 5001 Market
(1, 100, 'STATION_MARKET', 5001, NULL),
(2, 50, 'STATION_MARKET', 5001, NULL),
(3, 10, 'STATION_MARKET', 5001, NULL),
(4, 5, 'STATION_MARKET', 5001, NULL),
-- Station 5002 Market
(1, 100, 'STATION_MARKET', 5002, NULL),
(2, 50, 'STATION_MARKET', 5002, NULL),
(3, 10, 'STATION_MARKET', 5002, NULL),
(4, 5, 'STATION_MARKET', 5002, NULL),
-- Station 5003 Market
(1, 100, 'STATION_MARKET', 5003, NULL),
(2, 50, 'STATION_MARKET', 5003, NULL),
(3, 10, 'STATION_MARKET', 5003, NULL),
(4, 5, 'STATION_MARKET', 5003, NULL),
-- Station 5004 Market
(1, 100, 'STATION_MARKET', 5004, NULL),
(2, 50, 'STATION_MARKET', 5004, NULL),
(3, 10, 'STATION_MARKET', 5004, NULL),
(4, 5, 'STATION_MARKET', 5004, NULL),
-- Station 5005 Market (Pirate Haven)
(1, 100, 'STATION_MARKET', 5005, NULL),
(2, 50, 'STATION_MARKET', 5005, NULL),
(3, 10, 'STATION_MARKET', 5005, NULL),
(4, 5, 'STATION_MARKET', 5005, NULL)
ON CONFLICT DO NOTHING;

-- Reset serial sequence for definitions table
SELECT setval('item_definitions_id_seq', (SELECT MAX(id) FROM item_definitions));
