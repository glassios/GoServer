import type { TurretDefinition } from '@/src/shared/types/turret';
import { validateTurretDefinition } from '@/src/shared/types/validateTurret';

export const TURRETS_FOLDER = 'prefubs/Turrets';

export function turretAssetPath(filename: string) {
  return `${TURRETS_FOLDER}/${filename}`;
}

export function filenameForTurretId(id: string) {
  const safe = id.trim().replace(/[^a-zA-Z0-9_-]/g, '_');
  return `${safe || 'turret'}.json`;
}

export async function listProjectTurretFiles(): Promise<string[]> {
  try {
    const res = await fetch('/api/prefubs/turrets');
    if (!res.ok) return [];
    const data = (await res.json()) as { files?: string[] };
    return data.files ?? [];
  } catch {
    return [];
  }
}

export async function saveTurretToProject(config: TurretDefinition): Promise<string> {
  const res = await fetch('/api/prefubs/turrets/save', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ config }),
  });
  const data = (await res.json()) as { ok?: boolean; filename?: string; error?: string };
  if (!res.ok || !data.ok) {
    throw new Error(data.error ?? `Save failed (${res.status})`);
  }
  return data.filename ?? filenameForTurretId(config.id);
}

export async function loadTurretFromProject(filename: string): Promise<TurretDefinition> {
  const res = await fetch(`/api/prefubs/turrets/${encodeURIComponent(filename)}`);
  if (!res.ok) {
    throw new Error(`Load failed (${res.status})`);
  }
  const parsed = (await res.json()) as unknown;
  const result = validateTurretDefinition(parsed);
  if ('errors' in result) {
    throw new Error(result.errors.join('; '));
  }
  return result.data;
}

export function downloadTurretJson(config: TurretDefinition) {
  const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filenameForTurretId(config.id);
  a.click();
  URL.revokeObjectURL(url);
}
