import type { EngineDefinition } from '@/src/shared/types/engine';
import { validateEngineDefinition } from '@/src/shared/types/validate';

/** Project folder: SpaceShip2d/prefubs/Engines */
export const ENGINES_FOLDER = 'prefubs/Engines';

export function engineAssetPath(filename: string) {
  return `${ENGINES_FOLDER}/${filename}`;
}

export function filenameForEngineId(id: string) {
  const safe = id.trim().replace(/[^a-zA-Z0-9_-]/g, '_');
  return `${safe || 'engine'}.json`;
}

export async function listProjectEngineFiles(): Promise<string[]> {
  try {
    const res = await fetch('/api/prefubs/engines');
    if (!res.ok) return [];
    const data = (await res.json()) as { files?: string[] };
    return data.files ?? [];
  } catch {
    return [];
  }
}

export async function saveEngineToProject(config: EngineDefinition): Promise<string> {
  const res = await fetch('/api/prefubs/engines/save', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ config }),
  });
  const data = (await res.json()) as { ok?: boolean; filename?: string; error?: string; path?: string };
  if (!res.ok || !data.ok) {
    throw new Error(data.error ?? `Save failed (${res.status})`);
  }
  return data.filename ?? filenameForEngineId(config.id);
}

export async function loadEngineFromProject(filename: string): Promise<EngineDefinition> {
  const res = await fetch(`/api/prefubs/engines/${encodeURIComponent(filename)}`);
  if (!res.ok) {
    throw new Error(`Load failed (${res.status})`);
  }
  const parsed = (await res.json()) as unknown;
  const result = validateEngineDefinition(parsed);
  if ('errors' in result) {
    throw new Error(result.errors.join('; '));
  }
  return result.data;
}

export async function loadEngineFromProjectUrl(assetPath: string): Promise<EngineDefinition> {
  const url = assetPath.startsWith('/') ? assetPath : `/${assetPath}`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const parsed = (await res.json()) as unknown;
  const result = validateEngineDefinition(parsed);
  if ('errors' in result) {
    throw new Error(result.errors.join('; '));
  }
  return result.data;
}

/** Fallback when dev API is unavailable (static build) */
export function downloadEngineJson(config: EngineDefinition) {
  const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filenameForEngineId(config.id);
  a.click();
  URL.revokeObjectURL(url);
}
