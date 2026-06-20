DROP TABLE IF EXISTS item_instances CASCADE;
DROP TABLE IF EXISTS item_definitions CASCADE;
DROP TABLE IF EXISTS fleet_ships CASCADE;
DROP TABLE IF EXISTS npcs CASCADE;
DROP TABLE IF EXISTS npc_behaviors CASCADE;
DROP TABLE IF EXISTS jump_gates CASCADE;
DROP TABLE IF EXISTS asteroids CASCADE;
DROP TABLE IF EXISTS stations CASCADE;
DROP TABLE IF EXISTS corporation_members CASCADE;
DROP TABLE IF EXISTS corporations CASCADE;
DROP TABLE IF EXISTS factions CASCADE;
DROP TABLE IF EXISTS characters CASCADE;
DROP TABLE IF EXISTS accounts CASCADE;

-- Create accounts table
CREATE TABLE accounts (
    id BIGSERIAL PRIMARY KEY,
    login VARCHAR(64) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create characters table
CREATE TABLE characters (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT UNIQUE NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name VARCHAR(64) UNIQUE NOT NULL,
    x REAL NOT NULL DEFAULT 0.0,
    y REAL NOT NULL DEFAULT 0.0,
    rotation REAL NOT NULL DEFAULT 0.0,
    credits BIGINT NOT NULL DEFAULT 1000,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create factions table
CREATE TABLE factions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(64) UNIQUE NOT NULL,
    description TEXT
);

-- Create corporations table
CREATE TABLE corporations (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    wallet BIGINT NOT NULL DEFAULT 0,
    founder_id BIGINT NOT NULL
);

-- Create corporation_members table
CREATE TABLE corporation_members (
    corp_id INT NOT NULL REFERENCES corporations(id) ON DELETE CASCADE,
    account_id BIGINT PRIMARY KEY,
    role VARCHAR(50) NOT NULL
);

-- Indexing for speed
CREATE INDEX idx_corp_members_corp_id ON corporation_members(corp_id);

-- Create stations table
CREATE TABLE stations (
    id BIGSERIAL PRIMARY KEY,
    system_id INT NOT NULL,
    name VARCHAR(64) NOT NULL,
    x REAL NOT NULL,
    y REAL NOT NULL,
    faction_id INT REFERENCES factions(id),
    wallet BIGINT NOT NULL DEFAULT 0
);

-- Create asteroids table
CREATE TABLE asteroids (
    id BIGSERIAL PRIMARY KEY,
    system_id INT NOT NULL,
    resource_type VARCHAR(32) NOT NULL,
    amount INT NOT NULL,
    x REAL NOT NULL,
    y REAL NOT NULL
);

-- Create jump_gates table
CREATE TABLE jump_gates (
    id BIGSERIAL PRIMARY KEY,
    system_id INT NOT NULL,
    x REAL NOT NULL,
    y REAL NOT NULL,
    target_system_id INT NOT NULL,
    target_x REAL NOT NULL,
    target_y REAL NOT NULL
);

-- Create npc_behaviors table
CREATE TABLE npc_behaviors (
    id VARCHAR(32) PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    description TEXT
);

-- Create npcs table
CREATE TABLE npcs (
    id BIGSERIAL PRIMARY KEY,
    system_id INT NOT NULL,
    name VARCHAR(64) NOT NULL,
    faction_id INT NOT NULL,
    corp_id INT NOT NULL REFERENCES corporations(id),
    x REAL NOT NULL,
    y REAL NOT NULL,
    behavior VARCHAR(32) NOT NULL DEFAULT 'Patrol' REFERENCES npc_behaviors(id)
);

-- Create fleet_ships table
CREATE TABLE fleet_ships (
    id BIGSERIAL PRIMARY KEY,
    owner_id BIGINT NOT NULL, -- references accounts.id or npcs.id
    owner_type VARCHAR(10) NOT NULL, -- 'player' or 'npc'
    ship_type VARCHAR(32) NOT NULL,
    health REAL NOT NULL,
    max_health REAL NOT NULL,
    shield REAL NOT NULL,
    max_shield REAL NOT NULL,
    cargo_capacity INT NOT NULL
);

-- Create Item Definitions table
CREATE TABLE item_definitions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL,      -- 'resource', 'material', 'module', 'ship', 'blueprint'
    stackable BOOLEAN NOT NULL DEFAULT true,
    volume REAL NOT NULL DEFAULT 0.0,
    meta_data JSONB DEFAULT '{}'::jsonb
);

-- Create Item Instances table
CREATE TABLE item_instances (
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
CREATE INDEX idx_item_instances_location ON item_instances(location_type, location_id);
CREATE INDEX idx_item_instances_owner ON item_instances(owner_id);

-- Seeding data
-- Seed Factions
INSERT INTO factions (id, name, description) VALUES
(1, 'Pirates', 'Space pirates targeting miners and explorers'),
(2, 'Miners', 'Industrial miners extracting valuable resources'),
(3, 'Galactic Guardians', 'Law enforcement and patrols protecting the sectors');

-- Seed NPC behaviors
INSERT INTO npc_behaviors (id, name, description) VALUES
('Mine', 'Mining', 'Mining resources from asteroids and depositing to corporate vault'),
('Escort', 'Escort', 'Following and protecting another fleet'),
('Patrol', 'Patrolling', 'Patrolling around home position'),
('Attack', 'Combat / Attack', 'Attacking hostile faction members and players (Stub)'),
('Defend', 'Defending', 'Defending an object or location');

-- Seed NPC Corporations
INSERT INTO corporations (id, name, wallet, founder_id) VALUES
(1, 'Galactic Guardians', 1000000, 0),
(2, 'Pirates', 50000, 0),
(3, 'Miners', 250000, 0);

-- Seed Stations
INSERT INTO stations (id, system_id, name, x, y, faction_id, wallet) VALUES
(5001, 1, 'Centauri Prime Station', -300, 200, 3, 100000),
(5002, 1, 'Sol Mining Outpost', 400, -300, 2, 100000),
(5005, 1, 'Pirate Haven', -800, -800, 1, 100000),
(5003, 2, 'Centauri Prime Station', -300, 200, 3, 100000),
(5004, 2, 'Sol Mining Outpost', 400, -300, 2, 100000);

-- Seed Asteroids
INSERT INTO asteroids (id, system_id, resource_type, amount, x, y) VALUES
(6001, 1, 'Iron', 1000, -150, 100),
(6002, 1, 'Iron', 1000, -100, 120),
(6003, 1, 'Titanium', 1000, 200, -200),
(6004, 1, 'Crystal', 1000, 500, 500),
(6005, 2, 'Iron', 1000, -150, 100),
(6006, 2, 'Iron', 1000, -100, 120),
(6007, 2, 'Titanium', 1000, 200, -200),
(6008, 2, 'Crystal', 1000, 500, 500);

-- Seed Jump Gates
INSERT INTO jump_gates (id, system_id, x, y, target_system_id, target_x, target_y) VALUES
(7001, 1, 2000, 2000, 2, -1800, -1800),
(7002, 2, -2000, -2000, 1, 1800, 1800);

-- Seed NPCs
INSERT INTO npcs (id, system_id, name, faction_id, corp_id, x, y, behavior) VALUES
(1001, 1, 'Guardian Patrol 1', 3, 1, -500, 500, 'Patrol'),
(1002, 1, 'Pirate Raider 1', 1, 2, 800, -800, 'Mine'),
(1003, 1, 'NPC Miner 1', 2, 3, -200, 150, 'Mine'),
(1004, 1, 'NPC Miner 2', 2, 3, 300, -150, 'Mine'),
(1005, 1, 'Pirate Raider 2', 1, 2, -900, -900, 'Escort'),
(2001, 2, 'Guardian Patrol 2', 3, 1, 500, -500, 'Defend'),
(2002, 2, 'Pirate Raider 3', 1, 2, -800, 800, 'Mine'),
(2003, 2, 'NPC Miner 3', 2, 3, 200, -250, 'Mine'),
(2004, 2, 'NPC Miner 4', 2, 3, -300, 300, 'Mine'),
(2005, 2, 'Pirate Raider 4', 1, 2, 900, 900, 'Patrol');

-- Seed Fleet Ships
INSERT INTO fleet_ships (owner_id, owner_type, ship_type, health, max_health, shield, max_shield, cargo_capacity) VALUES
-- Guardian Patrol 1 (fighter)
(1001, 'npc', 'patrol', 100, 100, 50, 50, 100),
-- Pirate Raider 1 (fighter)
(1002, 'npc', 'pirate', 60, 60, 20, 20, 50),
-- NPC Miner 1 (miner + fighter cargo helper)
(1003, 'npc', 'miner', 80, 80, 30, 30, 150),
(1003, 'npc', 'cargo_helper', 100, 100, 40, 40, 200),
-- NPC Miner 2 (miner)
(1004, 'npc', 'miner', 80, 80, 30, 30, 150),
-- Pirate Raider 2 (fighter)
(1005, 'npc', 'pirate', 60, 60, 20, 20, 50),
-- Guardian Patrol 2 (fighter)
(2001, 'npc', 'patrol', 100, 100, 50, 50, 100),
-- Pirate Raider 3 (fighter)
(2002, 'npc', 'pirate', 60, 60, 20, 20, 50),
-- NPC Miner 3 (miner)
(2003, 'npc', 'miner', 80, 80, 30, 30, 150),
-- NPC Miner 4 (miner)
(2004, 'npc', 'miner', 80, 80, 30, 30, 150),
-- Pirate Raider 4 (fighter)
(2005, 'npc', 'pirate', 60, 60, 20, 20, 50);

-- Seed Item Definitions
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
(14, 'EnergyCoils', 'material', true, 0.1, '{}'::jsonb);

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
-- Station 5005 Market
(1, 100, 'STATION_MARKET', 5005, NULL),
(2, 50, 'STATION_MARKET', 5005, NULL),
(3, 10, 'STATION_MARKET', 5005, NULL),
(4, 5, 'STATION_MARKET', 5005, NULL);

-- Reset serial/bigserial sequences for tables that have manually seeded IDs
SELECT setval('factions_id_seq', COALESCE((SELECT MAX(id) FROM factions), 1));
SELECT setval('corporations_id_seq', COALESCE((SELECT MAX(id) FROM corporations), 1));
SELECT setval('stations_id_seq', COALESCE((SELECT MAX(id) FROM stations), 1));
SELECT setval('npcs_id_seq', COALESCE((SELECT MAX(id) FROM npcs), 1));
SELECT setval('asteroids_id_seq', COALESCE((SELECT MAX(id) FROM asteroids), 1));
SELECT setval('jump_gates_id_seq', COALESCE((SELECT MAX(id) FROM jump_gates), 1));
SELECT setval('item_definitions_id_seq', COALESCE((SELECT MAX(id) FROM item_definitions), 1));
