import type { InputMap } from '@/src/shared/engine/resolveMode';

export interface GameInputState extends InputMap {
  w: boolean;
  a: boolean;
  s: boolean;
  d: boolean;
  fire: boolean;
  blaster: boolean;
}

export const gameInput: GameInputState = {
  w: false,
  a: false,
  s: false,
  d: false,
  fire: false,
  blaster: false,
};

export const gameUiState = {
  shieldActive: false,
  /** Ship-local XY — shared ripple refraction for hull, turrets, and shield hits. */
  rippleHits: [] as { x: number; y: number; time: number; scale: number }[],
};

/** @deprecated Use gameInput — kept for minimap / legacy imports during migration */
export const inputState = gameInput;

/** @deprecated Use gameUiState */
export const uiState = gameUiState;
