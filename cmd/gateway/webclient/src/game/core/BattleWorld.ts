import type { ShipEntity } from '@/src/game/entities/ShipEntity';

/** Arena registry for multiple ships (phase 2). */
export class BattleWorld {
  private readonly ships = new Map<string, ShipEntity>();

  addShip(entity: ShipEntity): void {
    this.ships.set(entity.id, entity);
  }

  removeShip(id: string): void {
    this.ships.delete(id);
  }

  getShip(id: string): ShipEntity | undefined {
    return this.ships.get(id);
  }

  getAllShips(): ShipEntity[] {
    return [...this.ships.values()];
  }

  tick(delta: number): void {
    void delta;
    // Phase 3: movement, weapons, collisions for all entities
  }

  clear(): void {
    this.ships.clear();
  }
}
