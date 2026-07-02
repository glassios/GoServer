import * as THREE from 'three';
import type { GameInputState } from '@/src/game/input/gameInput';
import type { ShipEntity } from '@/src/game/entities/ShipEntity';

const MAX_SPEED = 10;
const ACCEL = 15;
const FRICTION = 0.96;
const TURN_SPEED = 4 / 3;

export class ShipMovementSystem {
  static tick(entity: ShipEntity, delta: number, input: GameInputState): void {
    const speed = ACCEL * delta;
    const turnSpeed = TURN_SPEED * delta;

    if (input.a) entity.transform.rotation += turnSpeed;
    if (input.d) entity.transform.rotation -= turnSpeed;

    const thrust = input.w ? 1 : input.s ? -0.5 : 0;
    if (thrust !== 0) {
      const dir = new THREE.Vector2(
        Math.cos(entity.transform.rotation),
        Math.sin(entity.transform.rotation)
      );
      entity.velocity.add(dir.multiplyScalar(thrust * speed));
    }

    entity.velocity.multiplyScalar(FRICTION);
    if (entity.velocity.lengthSq() > MAX_SPEED * MAX_SPEED) {
      entity.velocity.normalize().multiplyScalar(MAX_SPEED);
    }

    entity.transform.x += entity.velocity.x * delta;
    entity.transform.y += entity.velocity.y * delta;
  }
}
