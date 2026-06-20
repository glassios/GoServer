-- Create ship_hulls table
CREATE TABLE IF NOT EXISTS ship_hulls (
    id SERIAL PRIMARY KEY,
    hull_id VARCHAR(64) UNIQUE NOT NULL,
    name VARCHAR(128) NOT NULL,
    base_hp REAL NOT NULL,
    base_armor REAL NOT NULL,
    base_shield_max REAL NOT NULL,
    shield_type VARCHAR(32) NOT NULL DEFAULT 'omni', -- front, omni, none
    shield_arc REAL NOT NULL DEFAULT 90.0,
    shield_efficiency REAL NOT NULL DEFAULT 1.0,
    base_max_speed REAL NOT NULL DEFAULT 80.0,
    base_turn_rate REAL NOT NULL DEFAULT 1.5,
    ordnance_points INT NOT NULL DEFAULT 100,
    weapon_slots JSONB NOT NULL DEFAULT '[]'::jsonb
);

-- Create weapon_definitions table
CREATE TABLE IF NOT EXISTS weapon_definitions (
    id SERIAL PRIMARY KEY,
    weapon_id VARCHAR(64) UNIQUE NOT NULL,
    name VARCHAR(128) NOT NULL,
    weapon_type VARCHAR(32) NOT NULL, -- BALLISTIC, ENERGY, MISSILE
    weapon_size VARCHAR(32) NOT NULL, -- SMALL, MEDIUM, LARGE
    op_cost INT NOT NULL DEFAULT 5,
    damage_per_shot REAL NOT NULL DEFAULT 10.0,
    damage_type VARCHAR(32) NOT NULL DEFAULT 'ENERGY', -- KINETIC, EXPLOSIVE, ENERGY, FRAGMENTATION
    flux_cost REAL NOT NULL DEFAULT 10.0,
    range REAL NOT NULL DEFAULT 500.0,
    cooldown REAL NOT NULL DEFAULT 1.0
);

-- Create hullmods table
CREATE TABLE IF NOT EXISTS hullmods (
    id SERIAL PRIMARY KEY,
    mod_id VARCHAR(64) UNIQUE NOT NULL,
    name VARCHAR(128) NOT NULL,
    op_cost_by_size JSONB NOT NULL DEFAULT '{"FRIGATE": 5, "DESTROYER": 10, "CRUISER": 15, "CAPITAL": 25}'::jsonb,
    modifiers JSONB NOT NULL DEFAULT '{}'::jsonb
);

-- Create ship_configurations table
CREATE TABLE IF NOT EXISTS ship_configurations (
    id BIGSERIAL PRIMARY KEY,
    owner_id BIGINT NOT NULL,
    owner_type VARCHAR(32) NOT NULL, -- player, npc
    hull_id INT NOT NULL REFERENCES ship_hulls(id) ON DELETE RESTRICT,
    custom_name VARCHAR(128) NOT NULL,
    fitted_weapons JSONB NOT NULL DEFAULT '{}'::jsonb, -- slot_id -> weapon_id
    fitted_hullmods JSONB NOT NULL DEFAULT '[]'::jsonb, -- array of mod_id strings
    vents INT NOT NULL DEFAULT 0,
    capacitors INT NOT NULL DEFAULT 0
);

-- Create character_fleets table (maps character/npc to configurations)
CREATE TABLE IF NOT EXISTS character_fleets (
    id BIGSERIAL PRIMARY KEY,
    owner_id BIGINT NOT NULL,
    owner_type VARCHAR(32) NOT NULL DEFAULT 'player', -- player, npc
    system_id INT NOT NULL DEFAULT 1,
    x REAL NOT NULL DEFAULT 0.0,
    y REAL NOT NULL DEFAULT 0.0,
    ship_ids JSONB NOT NULL DEFAULT '[]'::jsonb, -- array of ship_configurations.id
    UNIQUE (owner_id, owner_type)
);
