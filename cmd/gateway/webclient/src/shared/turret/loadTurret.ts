import type { TurretDefinition } from '@/src/shared/types/turret';
import { validateTurretDefinition } from '@/src/shared/types/validateTurret';
import { assetUrl } from '@/src/shared/assetUrl';
import { createDefaultTurretDefinition } from './gameDefaults';

export async function loadTurretDefinition(assetPath: string): Promise<TurretDefinition> {
  const url = assetUrl(assetPath);
  try {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const json = await res.json();
    const result = validateTurretDefinition(json);
    if ('errors' in result) {
      console.warn('Turret validation failed:', result.errors);
      return createDefaultTurretDefinition('beam');
    }
    return result.data;
  } catch (e) {
    console.warn('Failed to load turret asset, using default:', assetPath, e);
    return createDefaultTurretDefinition('beam');
  }
}

export function cloneTurretDefinition(config: TurretDefinition): TurretDefinition {
  return JSON.parse(JSON.stringify(config)) as TurretDefinition;
}
