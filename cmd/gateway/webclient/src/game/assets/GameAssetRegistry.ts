import type { ShipDefinition } from '@/src/shared/types/ship';
import type { TurretDefinition } from '@/src/shared/types/turret';
import { loadShipDefinition } from '@/src/shared/ship/loadShip';
import { loadTurretDefinition } from '@/src/shared/turret/loadTurret';
import { ShipEntity } from '@/src/game/entities/ShipEntity';

export class GameAssetRegistry {
  private readonly shipDefinitions = new Map<string, ShipDefinition>();
  private readonly turretDefinitions = new Map<string, TurretDefinition>();

  async getShipDefinition(assetPath: string): Promise<ShipDefinition> {
    const cached = this.shipDefinitions.get(assetPath);
    if (cached) return cached;
    const def = await loadShipDefinition(assetPath);
    this.shipDefinitions.set(assetPath, def);
    return def;
  }

  async getTurretDefinition(assetPath: string): Promise<TurretDefinition> {
    const cached = this.turretDefinitions.get(assetPath);
    if (cached) return cached;
    const def = await loadTurretDefinition(assetPath);
    this.turretDefinitions.set(assetPath, def);
    return def;
  }

  async createShipEntity(assetPath: string): Promise<ShipEntity> {
    const definition = await this.getShipDefinition(assetPath);
    return ShipEntity.fromDefinition(definition, this);
  }

  clearCache(): void {
    this.shipDefinitions.clear();
    this.turretDefinitions.clear();
  }
}

export const gameAssetRegistry = new GameAssetRegistry();
