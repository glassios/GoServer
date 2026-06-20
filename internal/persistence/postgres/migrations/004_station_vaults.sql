-- Add cargo and wallet fields to stations table
ALTER TABLE stations ADD COLUMN IF NOT EXISTS cargo JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE stations ADD COLUMN IF NOT EXISTS wallet BIGINT NOT NULL DEFAULT 0;

-- Update default seeded stations with initial market stock and wallet credits
UPDATE stations SET 
    cargo = '{"Iron": 100, "Titanium": 50, "Crystal": 10, "RareGas": 5}'::jsonb,
    wallet = 100000
WHERE id IN (5001, 5002, 5003, 5004, 5005);

-- Create player vaults (hangars) at stations
CREATE TABLE IF NOT EXISTS player_station_vaults (
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    station_id BIGINT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
    items JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (account_id, station_id)
);

-- Create corporation vaults at stations
CREATE TABLE IF NOT EXISTS corporation_station_vaults (
    corp_id INT NOT NULL REFERENCES corporations(id) ON DELETE CASCADE,
    station_id BIGINT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
    items JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (corp_id, station_id)
);

-- Indexing for fast lookups
CREATE INDEX IF NOT EXISTS idx_player_station_vaults_station ON player_station_vaults(station_id);
CREATE INDEX IF NOT EXISTS idx_corporation_station_vaults_station ON corporation_station_vaults(station_id);
