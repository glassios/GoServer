import type { WorldTransform } from '@/src/shared/engine/simulator';

export class Transform {
  constructor(
    public x = 0,
    public y = 0,
    public z = 0,
    public rotation = 0
  ) {}

  toWorldTransform(): WorldTransform {
    return { x: this.x, y: this.y, z: this.z, rotation: this.rotation };
  }
}
