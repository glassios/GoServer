-- 010_fleet_tactics.sql
-- Phase 1.5: persist per-ship combat tactics (role + strategy) on fleet ships.
ALTER TABLE fleet_ships ADD COLUMN IF NOT EXISTS role     VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE fleet_ships ADD COLUMN IF NOT EXISTS strategy VARCHAR(16) NOT NULL DEFAULT '';
