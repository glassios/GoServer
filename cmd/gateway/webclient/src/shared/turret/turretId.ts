export function buildTurretId(name: string): string {
  const slug = name
    .trim()
    .replace(/\s+/g, '_')
    .replace(/[^a-zA-Z0-9_-]/g, '');
  return `${slug || 'turret'}_v0`;
}

export function turretIdMatchesAuto(config: { id: string; name: string }): boolean {
  return config.id === buildTurretId(config.name);
}
