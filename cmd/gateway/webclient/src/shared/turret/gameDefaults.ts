import type {
  BeamConfig,
  BeamEmitterSlot,
  BlasterConfig,
  BlasterMuzzleSlot,
  TurretDefinition,
  TurretKind,
  TurretPivot,
  TurretRotation,
  TurretRotationOrigin,
  TurretVisual,
} from '@/src/shared/types/turret';
import { SCHEMA_VERSION } from '@/src/shared/types/schema';
import { buildTurretId } from './turretId';
import { sanitizeRotation } from './turretRotation';

function defaultBeamWeaponParams(): Omit<BeamEmitterSlot, 'id' | 'localPosition' | 'lightPosition'> {
  return {
    beamLength: 50,
    beamWidth: 0.08,
    ramp: { rampUp: 3, rampDown: 5 },
    intensity: {
      lightMultiplier: 15,
      jitterShaderMin: 0.8,
      jitterShaderMax: 1.2,
      jitterLightExtra: 5,
    },
    colors: {
      core: { r: 1, g: 0.8, b: 0.8 },
      glow: { r: 1, g: 0, b: 0.1 },
    },
    shader: { alphaPowX: 3, alphaPowY: 2, glowMix: 1.5 },
    pointLight: {
      enabled: true,
      color: '#ff0000',
      distance: 0.8,
      decay: 2,
    },
  };
}

function defaultBlasterWeaponParams(): Omit<BlasterMuzzleSlot, 'id' | 'localPosition' | 'lightPosition'> {
  return {
    fireInterval: 0.08,
    boltSpeed: 23.3,
    boltLifetime: 1.0,
    maxBolts: 100,
    boltSize: [0.2, 0.08],
    colors: {
      core: { r: 0.6, g: 1, b: 0.6 },
      glow: { r: 0.1, g: 1, b: 0.2 },
    },
    shader: {
      alphaPowX: 3,
      alphaPowY: 2,
      glowMix: 1.5,
      colorMultiply: 2.5,
    },
    pointLights: {
      color: '#66ff66',
      flashIntensity: 15,
      decayLerp: 0.4,
      distance: 1,
      decay: 2,
    },
  };
}

export function createGameDefaultBeamEmitterSlot(id = 'emitter-1'): BeamEmitterSlot {
  return {
    id,
    localPosition: { x: 1.5, y: 0, z: 0 },
    lightPosition: { x: 1.8, y: 0, z: 0.5 },
    ...defaultBeamWeaponParams(),
  };
}

export function createGameDefaultBlasterMuzzleSlot(id: string, y: number): BlasterMuzzleSlot {
  return {
    id,
    localPosition: { x: 1.95, y, z: 0 },
    lightPosition: { x: 1.8, y: y > 0 ? 0.6 : -0.6, z: 0.5 },
    ...defaultBlasterWeaponParams(),
  };
}

/** Values extracted from LaserBeam in src/game/App.tsx */
export function createGameDefaultBeamConfig(): BeamConfig {
  return {
    emitters: [createGameDefaultBeamEmitterSlot()],
  };
}

/** Values extracted from BlasterBolts + BlasterLights in src/game/App.tsx */
export function createGameDefaultBlasterConfig(): BlasterConfig {
  return {
    muzzles: [
      createGameDefaultBlasterMuzzleSlot('muzzle-left', 0.56),
      createGameDefaultBlasterMuzzleSlot('muzzle-right', -0.56),
    ],
  };
}

export function createDefaultBeamEmitterSlot(index: number, template?: BeamEmitterSlot): BeamEmitterSlot {
  const n = index + 1;
  const weapon = template
    ? {
        beamLength: template.beamLength,
        beamWidth: template.beamWidth,
        ramp: { ...template.ramp },
        intensity: { ...template.intensity },
        colors: { core: { ...template.colors.core }, glow: { ...template.colors.glow } },
        shader: { ...template.shader },
        pointLight: { ...template.pointLight },
      }
    : defaultBeamWeaponParams();
  return {
    id: `emitter-${n}`,
    localPosition: { x: 1.5 + index * 0.25, y: 0, z: 0 },
    lightPosition: { x: 1.8 + index * 0.25, y: 0, z: 0.5 },
    ...weapon,
  };
}

export function createDefaultBlasterMuzzleSlot(
  index: number,
  template?: BlasterMuzzleSlot
): BlasterMuzzleSlot {
  const n = index + 1;
  const side = index % 2 === 0 ? 1 : -1;
  const row = Math.floor(index / 2) + 1;
  const y = side * (0.56 + (row - 1) * 0.2);
  const weapon = template
    ? {
        fireInterval: template.fireInterval,
        boltSpeed: template.boltSpeed,
        boltLifetime: template.boltLifetime,
        maxBolts: template.maxBolts,
        boltSize: [template.boltSize[0], template.boltSize[1]] as [number, number],
        colors: { core: { ...template.colors.core }, glow: { ...template.colors.glow } },
        shader: { ...template.shader },
        pointLights: { ...template.pointLights },
      }
    : defaultBlasterWeaponParams();
  return {
    id: `muzzle-${n}`,
    localPosition: { x: 1.95, y, z: 0 },
    lightPosition: { x: 1.8, y, z: 0.5 },
    ...weapon,
  };
}

/** @deprecated Use createDefaultBeamEmitterSlot / createDefaultBlasterMuzzleSlot */
export function createDefaultTurretWeaponSlot(kind: 'beam' | 'blaster', index: number) {
  return kind === 'beam'
    ? createDefaultBeamEmitterSlot(index)
    : createDefaultBlasterMuzzleSlot(index);
}

export function createDefaultTurretVisual(): TurretVisual {
  return {
    size: [1.2, 1.2],
    texture: '/turret.png',
    normalMap: '/turret_n.png',
    roughness: 0.4,
    metalness: 0.6,
  };
}

export function createDefaultTurretPivot(): TurretPivot {
  return { x: 0, y: 0, z: 0 };
}

export function createDefaultRotationOrigin(): TurretRotationOrigin {
  return { x: 0, y: 0, z: 0 };
}

export function createDefaultTurretRotation(): TurretRotation {
  return sanitizeRotation({
    minAngle: -Math.PI / 2,
    maxAngle: Math.PI / 2,
    turnSpeed: 2.5,
  });
}

export function createDefaultTurretDefinition(kind: TurretKind): TurretDefinition {
  const name = kind === 'beam' ? 'Laser Turret' : 'Blaster Turret';
  return {
    schemaVersion: SCHEMA_VERSION,
    id: buildTurretId(name),
    name,
    kind,
    visual: createDefaultTurretVisual(),
    pivot: createDefaultTurretPivot(),
    rotationOrigin: createDefaultRotationOrigin(),
    rotation: createDefaultTurretRotation(),
    beam: kind === 'beam' ? createGameDefaultBeamConfig() : undefined,
    blaster: kind === 'blaster' ? createGameDefaultBlasterConfig() : undefined,
  };
}

export function applyTurretKindWeapon(
  config: TurretDefinition,
  kind: TurretKind
): TurretDefinition {
  return {
    ...config,
    kind,
    beam: kind === 'beam' ? createGameDefaultBeamConfig() : undefined,
    blaster: kind === 'blaster' ? createGameDefaultBlasterConfig() : undefined,
  };
}
