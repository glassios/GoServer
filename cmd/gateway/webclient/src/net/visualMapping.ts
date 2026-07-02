// Maps a server hull id (EntitySnapshot.ship_type) to a SpaceShip2d visual prefab for the
// high-fidelity battle renderer. SLICE 3 points every hull at the seed fighter prefab; SLICE 8
// repoints these to per-class go_*.json generated from the catalog (internal/domain/fitting_catalog.go).
const DEFAULT_PREFAB = 'prefubs/Ships/default-fighter.json';

const SHIP_PREFABS: Record<string, string> = {
  fighter: DEFAULT_PREFAB,
  patrol: DEFAULT_PREFAB,
  pirate: DEFAULT_PREFAB,
  miner: DEFAULT_PREFAB,
  cargo_helper: DEFAULT_PREFAB,
  interceptor: DEFAULT_PREFAB,
  destroyer: DEFAULT_PREFAB,
  cruiser: DEFAULT_PREFAB,
};

export function shipPrefabFor(shipType: string): string {
  return SHIP_PREFABS[shipType] ?? DEFAULT_PREFAB;
}
