import type { ShipWeaponSlot } from '@/src/shared/types/weapon';
import { turretAssetPath } from '@/src/shared/turret/turretStorage';

export function createDefaultShipWeaponSlot(
  index: number,
  turretFilename = 'default-beam.json'
): ShipWeaponSlot {
  const n = index + 1;
  return {
    id: `turret-${n}`,
    turretAsset: turretAssetPath(turretFilename),
    scale: 1,
    mount: {
      localPosition: { x: 1.2 + index * 0.4, y: index % 2 === 0 ? 0.35 : -0.35, z: 0 },
      rotation: 0,
    },
  };
}
