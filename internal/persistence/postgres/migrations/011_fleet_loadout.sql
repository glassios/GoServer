-- 011_fleet_loadout.sql
-- Phase 2: persist per-ship fitting (loadout) on fleet ships as JSON.
-- When loadout is '' (empty) the ship uses the stock loadout for its hull type.
ALTER TABLE fleet_ships ADD COLUMN IF NOT EXISTS loadout TEXT NOT NULL DEFAULT '';
