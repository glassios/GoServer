import type { EngineDefinition } from '@/src/shared/types/engine';
import { validateEngineDefinition } from '@/src/shared/types/validate';
import { assetUrl } from '@/src/shared/assetUrl';
import { createDefaultEngineDefinition } from './defaults';

export async function loadEngineDefinition(assetPath: string): Promise<EngineDefinition> {
  const url = assetUrl(assetPath);
  try {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const json = await res.json();
    const result = validateEngineDefinition(json);
    if ('data' in result) return result.data;
    console.warn('Engine validation failed:', result.errors);
    return createDefaultEngineDefinition();
  } catch (e) {
    console.warn('Failed to load engine asset, using default:', assetPath, e);
    return createDefaultEngineDefinition();
  }
}

export function cloneEngineDefinition(config: EngineDefinition): EngineDefinition {
  return JSON.parse(JSON.stringify(config)) as EngineDefinition;
}
