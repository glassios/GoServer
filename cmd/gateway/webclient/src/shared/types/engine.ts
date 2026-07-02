import type { SchemaVersion } from './schema';

export type EngineSide = 'left' | 'right' | 'center';

export type EngineModeId = 'off' | 'idle' | 'thrust' | 'reverse';

export const ENGINE_MODE_IDS: EngineModeId[] = ['off', 'idle', 'thrust', 'reverse'];

export interface Vec3 {
  x: number;
  y: number;
  z: number;
}

export interface FlameColorStop {
  position: number;
  rgba: [number, number, number, number];
}

export interface EngineModeConfig {
  emitRate: number;
  poolWeight: number;
  lifeMin: number;
  lifeMax: number;
  speedBase: number;
  speedVar: number;
  lateralVelocity: number;
}

export interface EngineDefinition {
  schemaVersion: SchemaVersion;
  id: string;
  name: string;
  mount: {
    side: EngineSide;
    localPosition: Vec3;
    lateralOffset: number;
    exhaustAngle: number;
  };
  flame: {
    stops: FlameColorStop[];
    canvasSize?: number;
  };
  particle: {
    maxCount: number;
    quadSize: number;
    opacity: number;
    backOffset: number;
    zOffset: number;
    positionSpread: number;
  };
  modes: Record<EngineModeId, EngineModeConfig>;
  pointLight: {
    enabled: boolean;
    color: string;
    position: Vec3;
    distance: number;
    decay: number;
    intensityMin: number;
    intensityMax: number;
    rampUp: number;
    rampDown: number;
  };
}
