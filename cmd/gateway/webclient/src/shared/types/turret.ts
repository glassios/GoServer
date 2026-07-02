import type { SchemaVersion } from './schema';

import type { Vec3 } from './engine';



export type TurretKind = 'beam' | 'blaster';



export interface RgbColor {

  r: number;

  g: number;

  b: number;

}



export interface TurretVisual {

  size: [number, number];

  texture?: string;

  normalMap?: string;

  roughness?: number;

  metalness?: number;

}



export interface TurretPivot {

  x: number;

  y: number;

  z: number;

}



/** Pivot on the turret sprite (turret-local space, origin = center of visual plane). */

export interface TurretRotationOrigin {

  x: number;

  y: number;

  z: number;

}



export interface TurretRotation {

  /** Radians. Arc (max − min) must be ≤ 2π (360°). */

  minAngle: number;

  maxAngle: number;

  turnSpeed: number;

}



export interface BeamShaderConfig {

  alphaPowX: number;

  alphaPowY: number;

  glowMix: number;

}



export interface BeamEmitterSlot {

  id: string;

  localPosition: Vec3;

  lightPosition?: Vec3;

  beamLength: number;

  beamWidth: number;

  ramp: {

    rampUp: number;

    rampDown: number;

  };

  intensity: {

    lightMultiplier: number;

    jitterShaderMin: number;

    jitterShaderMax: number;

    jitterLightExtra: number;

  };

  colors: {

    core: RgbColor;

    glow: RgbColor;

  };

  shader: BeamShaderConfig;

  pointLight: {

    enabled: boolean;

    color: string;

    distance: number;

    decay: number;

  };

}



export interface BeamConfig {

  emitters: BeamEmitterSlot[];

}



export interface BlasterShaderConfig {

  alphaPowX: number;

  alphaPowY: number;

  glowMix: number;

  colorMultiply: number;

}



export interface BlasterMuzzleSlot {

  id: string;

  localPosition: Vec3;

  lightPosition?: Vec3;

  fireInterval: number;

  boltSpeed: number;

  boltLifetime: number;

  maxBolts: number;

  boltSize: [number, number];

  colors: {

    core: RgbColor;

    glow: RgbColor;

  };

  shader: BlasterShaderConfig;

  pointLights: {

    color: string;

    flashIntensity: number;

    decayLerp: number;

    distance: number;

    decay: number;

  };

}



export interface BlasterConfig {

  muzzles: BlasterMuzzleSlot[];

}



/** @deprecated Use BeamEmitterSlot or BlasterMuzzleSlot */

export type TurretWeaponSlot = BeamEmitterSlot | BlasterMuzzleSlot;



export interface TurretDefinition {

  schemaVersion: SchemaVersion;

  id: string;

  name: string;

  kind: TurretKind;

  visual: TurretVisual;

  pivot: TurretPivot;

  rotationOrigin: TurretRotationOrigin;

  rotation: TurretRotation;

  beam?: BeamConfig;

  blaster?: BlasterConfig;

}


