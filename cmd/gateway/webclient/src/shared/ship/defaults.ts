import { DEFAULT_SHIP_ENGINE_SLOTS } from '@/src/shared/engine/defaults';
import type { ShipDefinition } from '@/src/shared/types/ship';
import { SCHEMA_VERSION } from '@/src/shared/types/schema';
import { buildShipId } from './shipId';

export const DEFAULT_SHIP_ID = buildShipId('Fighter Alpha');

export function createDefaultShipDefinition(): ShipDefinition {
  return {
    schemaVersion: SCHEMA_VERSION,
    id: DEFAULT_SHIP_ID,
    name: 'Fighter Alpha',
    hull: {
      size: [6, 2.5],
      texture: '/m1.png',
      normalMap: '/m1n.png',
      roughness: 0.4,
      metalness: 0.6,
    },
    engines: DEFAULT_SHIP_ENGINE_SLOTS.map((s) => ({ ...s, mountOverride: s.mountOverride ? { ...s.mountOverride } : undefined, inputBindings: s.inputBindings ? { ...s.inputBindings } : undefined })),
    weapons: [],
  };
}
