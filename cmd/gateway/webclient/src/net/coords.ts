// system_id boundary between the open world and battle instances. Instances are published on
// topic system.<instance_id>.output with instance_id >= 10000 (see instance_manager.go), so the
// client switches to the high-fidelity battle renderer above this threshold.
export const BATTLE_SYSTEM_THRESHOLD = 10000;

export function isBattleSystem(systemId: number): boolean {
  return systemId >= BATTLE_SYSTEM_THRESHOLD;
}

// Server world space ↔ render space mapping.
//
// We use an IDENTITY orientation (server +X → render +X, server +Y → render +Y, angle as-is)
// so position, velocity, and rotation stay mutually consistent. This makes the new client a
// vertical mirror of the legacy Canvas2D client (which treats +Y as "down"), which is fine —
// it is internally coherent. WASD therefore maps W → +Y (see useNetKeyboard).
//
// Server world coordinates are large (open world ±1800; battle arena ~6000) while ships render at
// ~6 units wide. The store applies a mode-dependent scale that shrinks world distances into a
// sensible render range. The battle scale is smaller so ~15 HD ships cluster legibly inside the
// observer camera's auto-fit frame. Tune empirically.
export const SPACE_WORLD_SCALE = 0.1;
export const BATTLE_WORLD_SCALE = 0.05;

export function worldScaleFor(systemId: number): number {
  return isBattleSystem(systemId) ? BATTLE_WORLD_SCALE : SPACE_WORLD_SCALE;
}

/** Speed (server units/sec) above which a ship's engines switch to the "thrust" plume. */
export const THRUST_SPEED_THRESHOLD = 5;
