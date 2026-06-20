-- Seed Factions
INSERT INTO factions (id, name, description) VALUES
(1, 'Pirates', 'Space pirates targeting miners and explorers'),
(2, 'Miners', 'Industrial miners extracting valuable resources'),
(3, 'Galactic Guardians', 'Law enforcement and patrols protecting the sectors')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

-- Seed NPC Corporations
INSERT INTO corporations (id, name, wallet, founder_id) VALUES
(1, 'Galactic Guardians', 1000000, 0),
(2, 'Pirates', 50000, 0),
(3, 'Miners', 250000, 0)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, wallet = EXCLUDED.wallet, founder_id = EXCLUDED.founder_id;

-- Upsert stations to preserve custom ones
INSERT INTO stations (id, system_id, name, x, y, faction_id) VALUES
(5001, 1, 'Centauri Prime Station', -300, 200, 3),
(5002, 1, 'Sol Mining Outpost', 400, -300, 2),
(5005, 1, 'Pirate Haven', -800, -800, 1),
(5003, 2, 'Centauri Prime Station', -300, 200, 3),
(5004, 2, 'Sol Mining Outpost', 400, -300, 2)
ON CONFLICT (id) DO UPDATE SET system_id = EXCLUDED.system_id, name = EXCLUDED.name, x = EXCLUDED.x, y = EXCLUDED.y, faction_id = EXCLUDED.faction_id;


-- Create asteroids table
CREATE TABLE IF NOT EXISTS asteroids (
    id BIGSERIAL PRIMARY KEY,
    system_id INT NOT NULL,
    resource_type VARCHAR(32) NOT NULL,
    amount INT NOT NULL,
    x REAL NOT NULL,
    y REAL NOT NULL
);

-- Upsert asteroids with hardcoded IDs to preserve custom ones
INSERT INTO asteroids (id, system_id, resource_type, amount, x, y) VALUES
(6001, 1, 'Iron', 1000, -150, 100),
(6002, 1, 'Iron', 1000, -100, 120),
(6003, 1, 'Titanium', 1000, 200, -200),
(6004, 1, 'Crystal', 1000, 500, 500),
(6005, 2, 'Iron', 1000, -150, 100),
(6006, 2, 'Iron', 1000, -100, 120),
(6007, 2, 'Titanium', 1000, 200, -200),
(6008, 2, 'Crystal', 1000, 500, 500)
ON CONFLICT (id) DO UPDATE SET system_id = EXCLUDED.system_id, resource_type = EXCLUDED.resource_type, amount = EXCLUDED.amount, x = EXCLUDED.x, y = EXCLUDED.y;

-- Create jump_gates table
CREATE TABLE IF NOT EXISTS jump_gates (
    id BIGSERIAL PRIMARY KEY,
    system_id INT NOT NULL,
    x REAL NOT NULL,
    y REAL NOT NULL,
    target_system_id INT NOT NULL,
    target_x REAL NOT NULL,
    target_y REAL NOT NULL
);

-- Upsert jump_gates with hardcoded IDs to preserve custom ones
INSERT INTO jump_gates (id, system_id, x, y, target_system_id, target_x, target_y) VALUES
(7001, 1, 2000, 2000, 2, -1800, -1800),
(7002, 2, -2000, -2000, 1, 1800, 1800)
ON CONFLICT (id) DO UPDATE SET system_id = EXCLUDED.system_id, x = EXCLUDED.x, y = EXCLUDED.y, target_system_id = EXCLUDED.target_system_id, target_x = EXCLUDED.target_x, target_y = EXCLUDED.target_y;

-- Create npc_behaviors table
CREATE TABLE IF NOT EXISTS npc_behaviors (
    id VARCHAR(32) PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    description TEXT
);

-- Seed NPC behaviors
INSERT INTO npc_behaviors (id, name, description) VALUES
('Mine', 'Mining', 'Mining resources from asteroids and depositing to corporate vault'),
('Escort', 'Escort', 'Following and protecting another fleet'),
('Patrol', 'Patrolling', 'Patrolling around home position'),
('Attack', 'Combat / Attack', 'Attacking hostile faction members and players (Stub)'),
('Defend', 'Defending', 'Defending an object or location')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

-- Create npcs table
CREATE TABLE IF NOT EXISTS npcs (
    id BIGSERIAL PRIMARY KEY,
    system_id INT NOT NULL,
    name VARCHAR(64) NOT NULL,
    faction_id INT NOT NULL,
    corp_id INT NOT NULL REFERENCES corporations(id),
    x REAL NOT NULL,
    y REAL NOT NULL,
    behavior VARCHAR(32) NOT NULL DEFAULT 'Patrol' REFERENCES npc_behaviors(id)
);

-- Ensure behavior column exists if table was already created in database
ALTER TABLE npcs ADD COLUMN IF NOT EXISTS behavior VARCHAR(32) NOT NULL DEFAULT 'Patrol' REFERENCES npc_behaviors(id);

-- Upsert npcs to preserve custom ones
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
(2005, 2, 'Pirate Raider 4', 1, 2, 900, 900, 'Patrol')
ON CONFLICT (id) DO UPDATE SET system_id = EXCLUDED.system_id, name = EXCLUDED.name, faction_id = EXCLUDED.faction_id, corp_id = EXCLUDED.corp_id, x = EXCLUDED.x, y = EXCLUDED.y, behavior = EXCLUDED.behavior;



-- Create fleet_ships table
CREATE TABLE IF NOT EXISTS fleet_ships (
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

-- Delete fleet ships only for the specific seeded NPCs to avoid wiping custom ones
DELETE FROM fleet_ships WHERE owner_type = 'npc' AND owner_id IN (1001, 1002, 1003, 1004, 1005, 2001, 2002, 2003, 2004, 2005);
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

-- Reset serial/bigserial sequences for tables that have manually seeded IDs
SELECT setval('factions_id_seq', COALESCE((SELECT MAX(id) FROM factions), 1));
SELECT setval('corporations_id_seq', COALESCE((SELECT MAX(id) FROM corporations), 1));
SELECT setval('stations_id_seq', COALESCE((SELECT MAX(id) FROM stations), 1));
SELECT setval('npcs_id_seq', COALESCE((SELECT MAX(id) FROM npcs), 1));
SELECT setval('asteroids_id_seq', COALESCE((SELECT MAX(id) FROM asteroids), 1));
SELECT setval('jump_gates_id_seq', COALESCE((SELECT MAX(id) FROM jump_gates), 1));

