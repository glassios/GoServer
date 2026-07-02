import type { TurretRotation } from '@/src/shared/types/turret';

export const MAX_ROTATION_ARC_RAD = Math.PI * 2;
export const MAX_ROTATION_ARC_DEG = 360;

/** Avoid float noise in UI after deg ↔ rad (e.g. -30 → -29.999…). */
const DEG_PRECISION = 1e6;

export function roundDeg(deg: number): number {
  return Math.round(deg * DEG_PRECISION) / DEG_PRECISION;
}

export function radToDeg(rad: number): number {
  return roundDeg((rad * 180) / Math.PI);
}

export function degToRad(deg: number): number {
  return (roundDeg(deg) * Math.PI) / 180;
}

/** Normalize degrees to [-180, 180]. */
export function normalizeDeg(deg: number): number {
  let d = deg % 360;
  if (d > 180) d -= 360;
  if (d < -180) d += 360;
  return roundDeg(d);
}

/** Clamp rotation limits: min ≤ max, arc span ≤ 360°. */
export function clampRotationRange(minAngle: number, maxAngle: number): { minAngle: number; maxAngle: number } {
  let min = minAngle;
  let max = maxAngle;
  if (max < min) {
    const t = min;
    min = max;
    max = t;
  }
  if (max - min > MAX_ROTATION_ARC_RAD) {
    max = min + MAX_ROTATION_ARC_RAD;
  }
  return { minAngle: min, maxAngle: max };
}

export function sanitizeRotation(rotation: TurretRotation): TurretRotation {
  const { minAngle, maxAngle } = clampRotationRange(rotation.minAngle, rotation.maxAngle);
  return { ...rotation, minAngle, maxAngle };
}

export function patchRotationFromDegrees(
  rotation: TurretRotation,
  minDeg: number,
  maxDeg: number
): TurretRotation {
  const minAngle = degToRad(normalizeDeg(minDeg));
  const maxAngle = degToRad(normalizeDeg(maxDeg));
  return sanitizeRotation({ ...rotation, minAngle, maxAngle });
}

export function centerAim(rotation: TurretRotation): number {
  return (rotation.minAngle + rotation.maxAngle) / 2;
}

export function rotationDegrees(rotation: TurretRotation): { minDeg: number; maxDeg: number; spanDeg: number } {
  const minDeg = normalizeDeg(radToDeg(rotation.minAngle));
  const maxDeg = normalizeDeg(radToDeg(rotation.maxAngle));
  const spanDeg = Math.min(radToDeg(rotation.maxAngle - rotation.minAngle), MAX_ROTATION_ARC_DEG);
  return { minDeg, maxDeg, spanDeg: Math.max(0, spanDeg) };
}
