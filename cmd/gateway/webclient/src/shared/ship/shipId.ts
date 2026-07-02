export function buildShipId(name: string): string {
  const slug = name
    .trim()
    .replace(/\s+/g, '_')
    .replace(/[^a-zA-Z0-9_-]/g, '');
  return `${slug || 'ship'}_v0`;
}

export function withAutoShipId(config: { id: string; name: string }): string {
  return buildShipId(config.name);
}

export function shipIdMatchesAuto(config: { id: string; name: string }): boolean {
  return config.id === buildShipId(config.name);
}
