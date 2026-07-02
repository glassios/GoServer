const ID_VERSION = 'v0';

/** Name_v0 — auto id from display name (side is set on ship slots, not in engine editor) */
export function buildEngineId(name: string, version = ID_VERSION): string {
  const namePart =
    name
      .trim()
      .replace(/\s+/g, '_')
      .replace(/[^a-zA-Z0-9_-]/g, '')
      .replace(/_+/g, '_')
      .replace(/^_|_$/g, '') || 'engine';

  return `${namePart}_${version}`;
}

export function withAutoEngineId<T extends { id: string; name: string }>(config: T): T {
  return { ...config, id: buildEngineId(config.name) };
}

export function engineIdMatchesAuto(config: { id: string; name: string }): boolean {
  return config.id === buildEngineId(config.name);
}
