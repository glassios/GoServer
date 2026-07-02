import * as THREE from 'three';
import type { ShipDefinition, ShipEngineSlot } from '@/src/shared/types/ship';
import type { WorldTransform } from '@/src/shared/engine/simulator';
import type { GameAssetRegistry } from '@/src/game/assets/GameAssetRegistry';
import type { GameInputState } from '@/src/game/input/gameInput';
import { Transform } from '@/src/game/core/Transform';
import { TurretMount } from '@/src/game/entities/TurretMount';
import { ShipMovementSystem } from '@/src/game/systems/ShipMovementSystem';

export class ShipEntity {
  readonly id: string;
  readonly definition: ShipDefinition;
  readonly engineSlots: ShipEngineSlot[];
  readonly turretMounts: TurretMount[];
  readonly transform = new Transform();
  readonly velocity = new THREE.Vector2(0, 0);

  private constructor(
    definition: ShipDefinition,
    engineSlots: ShipEngineSlot[],
    turretMounts: TurretMount[]
  ) {
    this.id = definition.id;
    this.definition = definition;
    this.engineSlots = engineSlots;
    this.turretMounts = turretMounts;
  }

  static async fromDefinition(
    definition: ShipDefinition,
    registry: GameAssetRegistry
  ): Promise<ShipEntity> {
    const turretMounts: TurretMount[] = [];
    for (const slot of definition.weapons) {
      turretMounts.push(await TurretMount.create(slot, registry));
    }
    return new ShipEntity(definition, [...definition.engines], turretMounts);
  }

  getWorldTransform(): WorldTransform {
    return this.transform.toWorldTransform();
  }

  tickMovement(delta: number, input: GameInputState): void {
    ShipMovementSystem.tick(this, delta, input);
    for (const mount of this.turretMounts) {
      mount.updateAimFromHull(this.transform.rotation);
    }
  }

  setWeaponFiring(firing: boolean): void {
    for (const mount of this.turretMounts) {
      mount.setFiring(firing);
    }
  }
}
