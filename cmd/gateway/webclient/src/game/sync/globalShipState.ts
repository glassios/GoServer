import type { ShipEntity } from '@/src/game/entities/ShipEntity';

/** World position synced each frame for minimap and camera helpers. */
export const globalShipState = {
  x: 0,
  y: 0,
  rotation: 0,
};

/** @deprecated Alias for legacy App.tsx references */
export const shipState = globalShipState;

export function syncGlobalShipState(entity: ShipEntity): void {
  globalShipState.x = entity.transform.x;
  globalShipState.y = entity.transform.y;
  globalShipState.rotation = entity.transform.rotation;
}
