import type { EngineDefinition } from '@/src/shared/types/engine';
import type { ShipEngineSlot } from '@/src/shared/types/ship';
import { SCHEMA_VERSION } from '@/src/shared/types/schema';
import { buildEngineId, withAutoEngineId } from './engineId';

export const DEFAULT_ENGINE_ID = buildEngineId('Ion Thruster');

export function createDefaultEngineDefinition(): EngineDefinition {
  return withAutoEngineId({
    schemaVersion: SCHEMA_VERSION,
    id: DEFAULT_ENGINE_ID,
    name: 'Ion Thruster',
    mount: {
      side: 'center',
      localPosition: { x: 0, y: 0, z: 0 },
      lateralOffset: 0,
      exhaustAngle: 0,
    },
    flame: {
      canvasSize: 64,
      stops: [
        { position: 0, rgba: [255, 255, 255, 1] },
        { position: 0.2, rgba: [100, 200, 255, 0.8] },
        { position: 0.5, rgba: [0, 100, 255, 0.3] },
        { position: 1, rgba: [0, 50, 255, 0] },
      ],
    },
    particle: {
      maxCount: 500,
      quadSize: 1.2,
      opacity: 0.9,
      backOffset: 3.0,
      zOffset: -0.5,
      positionSpread: 0.05,
    },
    modes: {
      off: {
        emitRate: 0,
        poolWeight: 0,
        lifeMin: 0.05,
        lifeMax: 0.13,
        speedBase: 0,
        speedVar: 0,
        lateralVelocity: 0,
      },
      idle: {
        emitRate: 75,
        poolWeight: 1,
        lifeMin: 0.0125,
        lifeMax: 0.0325,
        speedBase: 0.25,
        speedVar: 0.25,
        lateralVelocity: 0.1,
      },
      thrust: {
        emitRate: 300,
        poolWeight: 4,
        lifeMin: 0.05,
        lifeMax: 0.13,
        speedBase: 6,
        speedVar: 4,
        lateralVelocity: 0.4,
      },
      reverse: {
        emitRate: 150,
        poolWeight: 2,
        lifeMin: 0.04,
        lifeMax: 0.1,
        speedBase: 4,
        speedVar: 2,
        lateralVelocity: 0.3,
      },
    },
    pointLight: {
      enabled: true,
      color: '#00e5ff',
      position: { x: -2.6, y: 0.5, z: 0.5 },
      distance: 10,
      decay: 2,
      intensityMin: 15,
      intensityMax: 35,
      rampUp: 0.4,
      rampDown: 0.2,
    },
  });
}

/** Four nozzle slots matching current game ship layout */
export const DEFAULT_SHIP_ENGINE_SLOTS: ShipEngineSlot[] = [
  {
    id: 'left-outer',
    engineAsset: 'prefubs/Engines/default-ion.json',
    mountOverride: { side: 'left', lateralOffset: -0.645 },
    inputBindings: { thrust: ['w'], reverse: ['s'], turnAssistLeft: ['d'] },
  },
  {
    id: 'left-inner',
    engineAsset: 'prefubs/Engines/default-ion.json',
    mountOverride: { side: 'left', lateralOffset: -0.38 },
    inputBindings: { thrust: ['w'], reverse: ['s'], turnAssistLeft: ['d'] },
  },
  {
    id: 'right-inner',
    engineAsset: 'prefubs/Engines/default-ion.json',
    mountOverride: { side: 'right', lateralOffset: 0.38 },
    inputBindings: { thrust: ['w'], reverse: ['s'], turnAssistRight: ['a'] },
  },
  {
    id: 'right-outer',
    engineAsset: 'prefubs/Engines/default-ion.json',
    mountOverride: { side: 'right', lateralOffset: 0.645 },
    inputBindings: { thrust: ['w'], reverse: ['s'], turnAssistRight: ['a'] },
  },
];

export function mergeEngineMount(
  base: EngineDefinition,
  override?: Partial<EngineDefinition['mount']>
): EngineDefinition['mount'] {
  return {
    ...base.mount,
    ...override,
    localPosition: {
      ...base.mount.localPosition,
      ...override?.localPosition,
    },
  };
}
