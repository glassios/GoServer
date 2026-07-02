import type { TurretDefinition } from '@/src/shared/types/turret';
import type { ShipWeaponSlot } from '@/src/shared/types/weapon';
import type { GameAssetRegistry } from '@/src/game/assets/GameAssetRegistry';

export class TurretMount {
  readonly slot: ShipWeaponSlot;
  definition: TurretDefinition | null = null;
  aimAngle = 0;
  firing = false;

  constructor(slot: ShipWeaponSlot) {
    this.slot = slot;
  }

  static async create(slot: ShipWeaponSlot, registry: GameAssetRegistry): Promise<TurretMount> {
    const mount = new TurretMount(slot);
    mount.definition = await registry.getTurretDefinition(slot.turretAsset);
    return mount;
  }

  get scale(): number {
    return this.slot.scale > 0 ? this.slot.scale : 1;
  }

  get worldAim(): number {
    return this.slot.mount.rotation + this.aimAngle;
  }

  setFiring(value: boolean): void {
    this.firing = value;
  }

  /** Aim relative to mount base follows hull heading, clamped to turret arc. */
  updateAimFromHull(hullRotation: number): void {
    if (!this.definition) return;
    const relative = hullRotation - this.slot.mount.rotation;
    const { minAngle, maxAngle } = this.definition.rotation;
    this.aimAngle = Math.min(maxAngle, Math.max(minAngle, relative));
  }
}
