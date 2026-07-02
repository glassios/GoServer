// Wire types for the gateway WebSocket JSON protocol.
//
// The gateway marshals protobuf with protojson { UseProtoNames: true, EmitUnpopulated: true }:
//   - keys are snake_case proto names
//   - EVERY field is always present (zero values emitted) — so compare values, not presence
//   - 64-bit ints (uint64/int64) are emitted as JSON STRINGS (entity_id, target_id, tick, credits)
//   - 32-bit ints / floats / bools are emitted as JSON numbers / booleans

/** One entity as it arrives in a world snapshot. IDs are strings (proto uint64). */
export interface RawEntitySnapshot {
  entity_id: string;
  entity_type: number;
  x: number;
  y: number;
  rotation: number;
  vx: number;
  vy: number;
  hp: number;
  max_hp: number;
  shield: number;
  max_shield: number;
  name: string;
  faction_id: number;
  target_id: string;
  is_shooting: boolean;
  ship_type: string;
  // Phase 1 Starsector combat
  armor: number;
  max_armor: number;
  flux: number;
  max_flux: number;
  overloaded: boolean;
  venting: boolean;
  last_damage_type: string;
  shots_fired: number;
  // Phase 1.5 roles & tactics
  role: string;
  strategy: string;
  assist_target_id: string;
  assist_type: string;
}

export interface WorldSnapshot {
  tick: string;
  entities: RawEntitySnapshot[];
}

/** Broadcast wrapper sent to every WS client every server tick. */
export interface WorldSnapshotMessage {
  system_id: number;
  snapshot: WorldSnapshot;
}

export interface AuthResponseMessage {
  type: 'auth_response';
  data: { success: boolean; entity_id: string; error_message?: string };
}

export interface SystemTransitionMessage {
  type: 'system_transition';
  system_id: number;
}

// domain.EntityType values (see internal/domain/entity.go). Ships = Player(0) or NPC(1).
export const ENTITY_TYPE_PLAYER = 0;
export const ENTITY_TYPE_NPC = 1;

/** True for entities we render as ships (player or NPC), driving hull/engine/turret visuals. */
export function isShipEntity(entityType: number): boolean {
  return entityType === ENTITY_TYPE_PLAYER || entityType === ENTITY_TYPE_NPC;
}
