import type { ShipDefinition } from '@/src/shared/types/ship';
import { validateShipDefinition } from '@/src/shared/types/validate';
import { assetUrl } from '@/src/shared/assetUrl';
import { createDefaultShipDefinition } from './defaults';

export function cloneShipDefinition(config: ShipDefinition): ShipDefinition {
  return JSON.parse(JSON.stringify(config)) as ShipDefinition;
}

export async function loadShipDefinition(assetPath: string): Promise<ShipDefinition> {
  const url = assetUrl(assetPath);
  try {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const json = await res.json();
    const result = validateShipDefinition(json);
    if ('data' in result) return result.data;
    console.warn('Ship validation failed:', result.errors);
    return createDefaultShipDefinition();
  } catch (e) {
    console.warn('Failed to load ship asset, using default:', assetPath, e);
    return createDefaultShipDefinition();
  }
}
