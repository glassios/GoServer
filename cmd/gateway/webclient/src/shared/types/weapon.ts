import type { SchemaVersion } from './schema';
import type { Vec3 } from './engine';

/** Turret mount on ship hull (sprite-local plane). */
export interface ShipWeaponSlot {
  id: string;
  /** Path to turret JSON, e.g. prefubs/Turrets/default-beam.json */
  turretAsset: string;
  /** Uniform scale relative to turret JSON size (1 = as authored). */
  scale: number;
  mount: {
    /** Ship hull-local position — turret pivot attachment point. */
    localPosition: Vec3;
    /** Base azimuth in radians (turret aim = 0). */
    rotation: number;
  };
}

export interface WeaponDefinition {
  schemaVersion: SchemaVersion;
  id: string;
  name: string;
}
