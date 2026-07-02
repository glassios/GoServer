import type { ShipDefinition } from '@/src/shared/types/ship';
import { validateShipDefinition } from '@/src/shared/types/validate';

export const SHIPS_FOLDER = 'prefubs/Ships';

export function filenameForShipId(id: string) {
  const safe = id.trim().replace(/[^a-zA-Z0-9_-]/g, '_');
  return `${safe || 'ship'}.json`;
}

export async function listProjectShipFiles(): Promise<string[]> {
  try {
    const res = await fetch('/api/prefubs/ships');
    if (!res.ok) return [];
    const data = (await res.json()) as { files?: string[] };
    return data.files ?? [];
  } catch {
    return [];
  }
}

export async function saveShipToProject(config: ShipDefinition): Promise<string> {
  const res = await fetch('/api/prefubs/ships/save', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ config }),
  });
  const data = (await res.json()) as { ok?: boolean; filename?: string; error?: string };
  if (!res.ok || !data.ok) {
    throw new Error(data.error ?? `Save failed (${res.status})`);
  }
  return data.filename ?? filenameForShipId(config.id);
}

export async function loadShipFromProject(filename: string): Promise<ShipDefinition> {
  const res = await fetch(`/api/prefubs/ships/${encodeURIComponent(filename)}`);
  if (!res.ok) {
    throw new Error(`Load failed (${res.status})`);
  }
  const parsed = (await res.json()) as unknown;
  const result = validateShipDefinition(parsed);
  if ('errors' in result) {
    throw new Error(result.errors.join('; '));
  }
  return result.data;
}

export function downloadShipJson(config: ShipDefinition) {
  const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filenameForShipId(config.id);
  a.click();
  URL.revokeObjectURL(url);
}
