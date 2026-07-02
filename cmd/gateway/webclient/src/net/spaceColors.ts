// Schematic colour vocabulary for the open-world view, mirroring the legacy Canvas2D client.
import { ENTITY_TYPE_PLAYER } from './types';
import type { RawEntitySnapshot } from './types';

/** Ship icon colour: local player cyan, other players green, NPCs by name (pirate/patrol/miner). */
export function shipColor(ent: RawEntitySnapshot, isLocal: boolean): string {
  if (isLocal) return '#00f2fe';
  if (ent.entity_type === ENTITY_TYPE_PLAYER) return '#39ff14';
  const n = ent.name || '';
  if (/Pirate|Outlaw|Raider/i.test(n)) return '#ff007f';
  if (/Patrol|Enforcer|Police/i.test(n)) return '#1e90ff';
  return '#ff9f1c'; // miner / civilian / unknown NPC
}

export const SPACE_COLORS = {
  asteroid: '#8a8f98',
  station: '#00d0ff',
  jumpGate: '#ff9f1c',
  loot: '#ffae42',
  combatMarker: '#ff007f',
  spaceBase: '#10b981',
} as const;
