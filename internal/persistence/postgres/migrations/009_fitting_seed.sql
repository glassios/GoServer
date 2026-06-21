-- 009_fitting_seed.sql
-- Seeds the (previously empty) Starsector fitting catalog created in 007.
-- This MUST stay in sync with the canonical code catalog in
-- internal/domain/fitting_catalog.go (numeric hull ids, weapon_id and mod_id strings,
-- and stats). Idempotent: re-running does nothing thanks to ON CONFLICT DO NOTHING.

-- ---------------------------------------------------------------------------
-- Ship hulls (explicit ids match domain.StockHulls[].ID)
-- ---------------------------------------------------------------------------
INSERT INTO ship_hulls (id, hull_id, name, base_hp, base_armor, base_shield_max, shield_type, shield_arc, shield_efficiency, base_max_speed, base_turn_rate, ordnance_points, weapon_slots) VALUES
(1, 'fighter', 'Fighter', 100, 40, 80, 'front', 120, 1.0, 120, 1.8, 60,
 '[{"slot_id":"WS1","size":"SMALL","type":"ENERGY","mount":"HARDPOINT","x":12,"y":0,"angle":0},
   {"slot_id":"WS2","size":"SMALL","type":"BALLISTIC","mount":"TURRET","x":-4,"y":6,"angle":0}]'::jsonb),
(2, 'patrol', 'Patrol Cutter', 140, 70, 120, 'omni', 180, 1.0, 100, 1.5, 90,
 '[{"slot_id":"WS1","size":"MEDIUM","type":"ENERGY","mount":"TURRET","x":8,"y":0,"angle":0},
   {"slot_id":"WS2","size":"SMALL","type":"BALLISTIC","mount":"TURRET","x":-6,"y":5,"angle":0},
   {"slot_id":"WS3","size":"SMALL","type":"BALLISTIC","mount":"TURRET","x":-6,"y":-5,"angle":0}]'::jsonb),
(3, 'pirate', 'Pirate Raider', 90, 40, 60, 'front', 90, 0.9, 90, 1.4, 55,
 '[{"slot_id":"WS1","size":"SMALL","type":"BALLISTIC","mount":"HARDPOINT","x":12,"y":0,"angle":0},
   {"slot_id":"WS2","size":"SMALL","type":"MISSILE","mount":"HARDPOINT","x":8,"y":6,"angle":0}]'::jsonb),
(4, 'miner', 'Mining Barge', 120, 60, 50, 'front', 120, 1.0, 60, 1.0, 40,
 '[{"slot_id":"WS1","size":"SMALL","type":"ENERGY","mount":"TURRET","x":6,"y":0,"angle":0}]'::jsonb),
(5, 'cargo_helper', 'Cargo Hauler', 140, 50, 40, 'front', 90, 1.0, 50, 0.8, 30,
 '[{"slot_id":"WS1","size":"SMALL","type":"BALLISTIC","mount":"TURRET","x":4,"y":0,"angle":0}]'::jsonb),
(6, 'interceptor', 'Interceptor', 70, 20, 50, 'front', 90, 1.0, 140, 2.2, 45,
 '[{"slot_id":"WS1","size":"SMALL","type":"ENERGY","mount":"HARDPOINT","x":10,"y":0,"angle":0}]'::jsonb),
(7, 'destroyer', 'Destroyer', 400, 200, 400, 'omni', 180, 1.0, 70, 0.9, 130,
 '[{"slot_id":"WS1","size":"MEDIUM","type":"BALLISTIC","mount":"TURRET","x":14,"y":0,"angle":0},
   {"slot_id":"WS2","size":"MEDIUM","type":"ENERGY","mount":"TURRET","x":-8,"y":8,"angle":0},
   {"slot_id":"WS3","size":"SMALL","type":"MISSILE","mount":"HARDPOINT","x":10,"y":-8,"angle":0}]'::jsonb),
(8, 'cruiser', 'Cruiser', 800, 400, 700, 'omni', 220, 1.0, 55, 0.6, 200,
 '[{"slot_id":"WS1","size":"LARGE","type":"BALLISTIC","mount":"HARDPOINT","x":18,"y":0,"angle":0},
   {"slot_id":"WS2","size":"MEDIUM","type":"ENERGY","mount":"TURRET","x":-6,"y":10,"angle":0},
   {"slot_id":"WS3","size":"MEDIUM","type":"ENERGY","mount":"TURRET","x":-6,"y":-10,"angle":0},
   {"slot_id":"WS4","size":"SMALL","type":"MISSILE","mount":"HARDPOINT","x":12,"y":12,"angle":0}]'::jsonb)
ON CONFLICT (hull_id) DO NOTHING;

-- Keep the SERIAL sequence ahead of the explicit ids we just inserted.
SELECT setval(pg_get_serial_sequence('ship_hulls', 'id'), (SELECT MAX(id) FROM ship_hulls));

-- ---------------------------------------------------------------------------
-- Weapon definitions (weapon_id matches domain.Weapon* constants)
-- ---------------------------------------------------------------------------
INSERT INTO weapon_definitions (weapon_id, name, weapon_type, weapon_size, op_cost, damage_per_shot, damage_type, flux_cost, range, cooldown) VALUES
('light_laser',       'Light Laser',      'ENERGY',    'SMALL',  5,   8,  'ENERGY',    6,  450, 0.4),
('ir_pulse_laser',    'IR Pulse Laser',   'ENERGY',    'SMALL',  6,  12,  'ENERGY',   10,  400, 0.6),
('light_autocannon',  'Light Autocannon', 'BALLISTIC', 'SMALL',  6,  10,  'KINETIC',   8,  500, 0.5),
('light_mortar',      'Light Mortar',     'BALLISTIC', 'SMALL',  5,  14,  'EXPLOSIVE', 9,  350, 0.8),
('swarmer_srm',       'Swarmer SRM',      'MISSILE',   'SMALL',  4,  20,  'EXPLOSIVE', 0,  600, 2.0),
('heavy_blaster',     'Heavy Blaster',    'ENERGY',    'MEDIUM', 12, 45,  'ENERGY',   45,  400, 1.0),
('heavy_mauler',      'Heavy Mauler',     'BALLISTIC', 'MEDIUM', 12, 40,  'EXPLOSIVE',30,  700, 1.2),
('hellbore',          'Hellbore Cannon',  'BALLISTIC', 'LARGE',  22, 100, 'EXPLOSIVE',80,  900, 2.5),
('mining_laser',      'Mining Laser',     'ENERGY',    'SMALL',  3,   5,  'ENERGY',    4,  300, 1.0)
ON CONFLICT (weapon_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Hullmods (mod_id matches domain.Hullmod* constants)
-- ---------------------------------------------------------------------------
INSERT INTO hullmods (mod_id, name, op_cost_by_size, modifiers) VALUES
('reinforced_bulkheads', 'Reinforced Bulkheads', '{"FRIGATE":5,"DESTROYER":10,"CRUISER":15,"CAPITAL":25}'::jsonb, '{"armor_mult":1.25}'::jsonb),
('augmented_engines',    'Augmented Engines',    '{"FRIGATE":5,"DESTROYER":10,"CRUISER":15,"CAPITAL":25}'::jsonb, '{"max_speed_mult":1.4,"turn_rate_mult":1.25}'::jsonb),
('hardened_shields',     'Hardened Shields',     '{"FRIGATE":5,"DESTROYER":10,"CRUISER":15,"CAPITAL":25}'::jsonb, '{"shield_max_mult":1.2,"shield_efficiency_mult":0.85}'::jsonb),
('flux_coil_adjunct',    'Flux Coil Adjunct',    '{"FRIGATE":5,"DESTROYER":10,"CRUISER":15,"CAPITAL":25}'::jsonb, '{"flux_dissipation_mult":1.3,"max_flux_mult":1.1}'::jsonb)
ON CONFLICT (mod_id) DO NOTHING;
