/** Spawn entry for a future battle.json arena config (phase 2). */
export interface BattleSpawnEntry {
  shipAsset: string;
  team: number;
  x: number;
  y: number;
  rotation?: number;
}

export interface BattleSpawnConfig {
  schemaVersion: number;
  spawns: BattleSpawnEntry[];
}
