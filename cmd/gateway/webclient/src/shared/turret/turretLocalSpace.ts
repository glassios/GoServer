/** Convert ship-offset from hull mount to turret-local (inverse of aim rotation). */
export function mountOffsetToTurretLocal(
  mountX: number,
  mountY: number,
  aimAngle: number
): { x: number; y: number } {
  const c = Math.cos(aimAngle);
  const s = Math.sin(aimAngle);
  return {
    x: mountX * c + mountY * s,
    y: -mountX * s + mountY * c,
  };
}

/** Convert turret-local point to offset from hull mount (forward aim rotation). */
export function turretLocalToMountOffset(
  localX: number,
  localY: number,
  aimAngle: number
): { x: number; y: number } {
  const c = Math.cos(aimAngle);
  const s = Math.sin(aimAngle);
  return {
    x: localX * c - localY * s,
    y: localX * s + localY * c,
  };
}
