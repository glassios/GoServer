import type { GameAssetRegistry } from '@/src/game/assets/GameAssetRegistry';
import { Transform } from '@/src/game/core/Transform';
import { ShipEntity } from '@/src/game/entities/ShipEntity';

export type TeamId = number;

export interface ShipSpawnOptions {
  assetPath: string;
  team?: TeamId;
  position?: { x: number; y: number; z?: number };
  rotation?: number;
}

/** Creates runtime ship instances for battles (phase 2+). */
export class ShipFactory {
  constructor(private readonly registry: GameAssetRegistry) {}

  async create(options: ShipSpawnOptions): Promise<ShipEntity> {
    const entity = await this.registry.createShipEntity(options.assetPath);
    if (options.position) {
      entity.transform.x = options.position.x;
      entity.transform.y = options.position.y;
      entity.transform.z = options.position.z ?? 0;
    }
    if (options.rotation !== undefined) {
      entity.transform.rotation = options.rotation;
    }
    return entity;
  }

  async createAt(
    assetPath: string,
    x: number,
    y: number,
    rotation = 0
  ): Promise<ShipEntity> {
    return this.create({ assetPath, position: { x, y }, rotation });
  }
}
