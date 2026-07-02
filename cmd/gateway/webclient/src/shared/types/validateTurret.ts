import { SCHEMA_VERSION } from './schema';

import type {

  BeamConfig,

  BeamEmitterSlot,

  BeamShaderConfig,

  BlasterConfig,

  BlasterMuzzleSlot,

  BlasterShaderConfig,

  RgbColor,

  TurretDefinition,

  TurretKind,

  TurretPivot,

  TurretRotation,

  TurretRotationOrigin,

  TurretVisual,

} from './turret';

import type { Vec3 } from './engine';

import { sanitizeRotation } from '@/src/shared/turret/turretRotation';



const KINDS: TurretKind[] = ['beam', 'blaster'];



function isObject(v: unknown): v is Record<string, unknown> {

  return typeof v === 'object' && v !== null && !Array.isArray(v);

}



function isNumber(v: unknown): v is number {

  return typeof v === 'number' && Number.isFinite(v);

}



function isString(v: unknown): v is string {

  return typeof v === 'string';

}



function validateVec3(v: unknown): Vec3 | null {

  if (!isObject(v)) return null;

  if (!isNumber(v.x) || !isNumber(v.y) || !isNumber(v.z)) return null;

  return { x: v.x, y: v.y, z: v.z };

}



function validateRgb(v: unknown): RgbColor | null {

  if (!isObject(v)) return null;

  if (!isNumber(v.r) || !isNumber(v.g) || !isNumber(v.b)) return null;

  return { r: v.r, g: v.g, b: v.b };

}



function validateVisual(v: unknown): TurretVisual | null {

  if (!isObject(v)) return null;

  if (!Array.isArray(v.size) || v.size.length !== 2 || !v.size.every(isNumber)) return null;

  const out: TurretVisual = { size: [v.size[0], v.size[1]] };

  if (v.texture !== undefined) {

    if (!isString(v.texture)) return null;

    out.texture = v.texture;

  }

  if (v.normalMap !== undefined) {

    if (!isString(v.normalMap)) return null;

    out.normalMap = v.normalMap;

  }

  if (v.roughness !== undefined) {

    if (!isNumber(v.roughness)) return null;

    out.roughness = v.roughness;

  }

  if (v.metalness !== undefined) {

    if (!isNumber(v.metalness)) return null;

    out.metalness = v.metalness;

  }

  return out;

}



function validatePivot(v: unknown): TurretPivot | null {

  const vec = validateVec3(v);

  if (!vec) return null;

  return vec;

}



function validateRotationOrigin(v: unknown): TurretRotationOrigin | null {

  const vec = validateVec3(v);

  if (!vec) return null;

  return vec;

}



const DEFAULT_ROTATION_ORIGIN: TurretRotationOrigin = { x: 0, y: 0, z: 0 };



function validateRotation(v: unknown): TurretRotation | null {

  if (!isObject(v)) return null;

  if (!isNumber(v.minAngle) || !isNumber(v.maxAngle) || !isNumber(v.turnSpeed)) return null;

  return sanitizeRotation({

    minAngle: v.minAngle,

    maxAngle: v.maxAngle,

    turnSpeed: v.turnSpeed,

  });

}



function readBeamShader(v: unknown, fallback: BeamShaderConfig): BeamShaderConfig | null {

  if (!isObject(v)) return fallback;

  if (!isNumber(v.alphaPowX) || !isNumber(v.alphaPowY) || !isNumber(v.glowMix)) return null;

  return { alphaPowX: v.alphaPowX, alphaPowY: v.alphaPowY, glowMix: v.glowMix };

}



function readBlasterShader(v: unknown, fallback: BlasterShaderConfig): BlasterShaderConfig | null {

  if (!isObject(v)) return fallback;

  if (

    !isNumber(v.alphaPowX) ||

    !isNumber(v.alphaPowY) ||

    !isNumber(v.glowMix) ||

    !isNumber(v.colorMultiply)

  ) {

    return null;

  }

  return {

    alphaPowX: v.alphaPowX,

    alphaPowY: v.alphaPowY,

    glowMix: v.glowMix,

    colorMultiply: v.colorMultiply,

  };

}



interface LegacyBeamShared {

  beamLength: number;

  beamWidth: number;

  ramp: BeamEmitterSlot['ramp'];

  intensity: BeamEmitterSlot['intensity'];

  colors: BeamEmitterSlot['colors'];

  shader: BeamShaderConfig;

  pointLight: BeamEmitterSlot['pointLight'];

}



function readLegacyBeamShared(v: Record<string, unknown>): LegacyBeamShared | null {

  if (!isObject(v.ramp) || !isObject(v.intensity) || !isObject(v.colors) || !isObject(v.shader)) {

    return null;

  }

  if (!isObject(v.pointLight)) return null;



  let beamLength: number | null = null;

  let beamWidth: number | null = null;

  if (isNumber(v.beamLength) && isNumber(v.beamWidth)) {

    beamLength = v.beamLength;

    beamWidth = v.beamWidth;

  } else if (isObject(v.emitter) && isNumber(v.emitter.beamLength) && isNumber(v.emitter.beamWidth)) {

    beamLength = v.emitter.beamLength;

    beamWidth = v.emitter.beamWidth;

  }

  if (beamLength === null || beamWidth === null) return null;



  if (!isNumber(v.ramp.rampUp) || !isNumber(v.ramp.rampDown)) return null;

  if (

    !isNumber(v.intensity.lightMultiplier) ||

    !isNumber(v.intensity.jitterShaderMin) ||

    !isNumber(v.intensity.jitterShaderMax) ||

    !isNumber(v.intensity.jitterLightExtra)

  ) {

    return null;

  }

  const core = validateRgb(v.colors.core);

  const glow = validateRgb(v.colors.glow);

  if (!core || !glow) return null;

  const shader = readBeamShader(v.shader, { alphaPowX: 3, alphaPowY: 2, glowMix: 1.5 });

  if (!shader) return null;

  if (typeof v.pointLight.enabled !== 'boolean' || !isString(v.pointLight.color)) return null;

  if (!isNumber(v.pointLight.distance) || !isNumber(v.pointLight.decay)) return null;



  return {

    beamLength,

    beamWidth,

    ramp: { rampUp: v.ramp.rampUp, rampDown: v.ramp.rampDown },

    intensity: {

      lightMultiplier: v.intensity.lightMultiplier,

      jitterShaderMin: v.intensity.jitterShaderMin,

      jitterShaderMax: v.intensity.jitterShaderMax,

      jitterLightExtra: v.intensity.jitterLightExtra,

    },

    colors: { core, glow },

    shader,

    pointLight: {

      enabled: v.pointLight.enabled,

      color: v.pointLight.color,

      distance: v.pointLight.distance,

      decay: v.pointLight.decay,

    },

  };

}



function validateBeamEmitterSlot(v: unknown, legacy?: LegacyBeamShared): BeamEmitterSlot | null {

  if (!isObject(v) || !isString(v.id) || !v.id.trim()) return null;

  const localPosition = validateVec3(v.localPosition);

  if (!localPosition) return null;



  const hasOwnWeapon =

    isNumber(v.beamLength) &&

    isNumber(v.beamWidth) &&

    isObject(v.ramp) &&

    isObject(v.intensity) &&

    isObject(v.colors) &&

    isObject(v.shader) &&

    isObject(v.pointLight);



  if (hasOwnWeapon) {

    const shared = readLegacyBeamShared(v);

    if (!shared) return null;

    const slot: BeamEmitterSlot = {

      id: v.id,

      localPosition,

      ...shared,

    };

    if (v.lightPosition !== undefined) {

      const lightPosition = validateVec3(v.lightPosition);

      if (!lightPosition) return null;

      slot.lightPosition = lightPosition;

    }

    return slot;

  }



  if (!legacy) return null;

  const slot: BeamEmitterSlot = {

    id: v.id,

    localPosition,

    ...legacy,

  };

  if (v.lightPosition !== undefined) {

    const lightPosition = validateVec3(v.lightPosition);

    if (!lightPosition) return null;

    slot.lightPosition = lightPosition;

  }

  return slot;

}



function migrateLegacyBeamEmitterList(v: Record<string, unknown>): BeamEmitterSlot[] | null {

  if (!isObject(v.emitter)) return null;

  const lp = validateVec3(v.emitter.localPosition);

  if (!lp) return null;

  const pl = isObject(v.pointLight) ? validateVec3(v.pointLight.position) : null;

  const legacy = readLegacyBeamShared(v);

  if (!legacy) return null;

  return [

    validateBeamEmitterSlot(

      {

        id: 'emitter-1',

        localPosition: lp,

        lightPosition: pl ?? { x: lp.x + 0.3, y: lp.y, z: lp.z + 0.5 },

        ...legacy,

      },

      legacy

    )!,

  ];

}



function validateBeam(v: unknown): BeamConfig | null {

  if (!isObject(v)) return null;



  const legacyShared = readLegacyBeamShared(v);

  let emitters: BeamEmitterSlot[] | null = null;



  if (Array.isArray(v.emitters)) {

    if (v.emitters.length === 0) return null;

    emitters = [];

    for (const item of v.emitters) {

      const slot = validateBeamEmitterSlot(item, legacyShared ?? undefined);

      if (!slot) return null;

      emitters.push(slot);

    }

  } else if (isObject(v.emitter)) {

    emitters = migrateLegacyBeamEmitterList(v);

  }



  if (!emitters) return null;

  return { emitters };

}



interface LegacyBlasterShared {

  fireInterval: number;

  boltSpeed: number;

  boltLifetime: number;

  maxBolts: number;

  boltSize: [number, number];

  colors: BlasterMuzzleSlot['colors'];

  shader: BlasterShaderConfig;

  pointLights: BlasterMuzzleSlot['pointLights'];

}



function readLegacyBlasterShared(v: Record<string, unknown>): LegacyBlasterShared | null {

  if (!isObject(v.colors) || !isObject(v.shader) || !isObject(v.pointLights)) return null;

  if (

    !isNumber(v.fireInterval) ||

    !isNumber(v.boltSpeed) ||

    !isNumber(v.boltLifetime) ||

    !isNumber(v.maxBolts)

  ) {

    return null;

  }

  if (!Array.isArray(v.boltSize) || v.boltSize.length !== 2 || !v.boltSize.every(isNumber)) return null;



  const core = validateRgb(v.colors.core);

  const glow = validateRgb(v.colors.glow);

  if (!core || !glow) return null;

  const shader = readBlasterShader(v.shader, {

    alphaPowX: 3,

    alphaPowY: 2,

    glowMix: 1.5,

    colorMultiply: 2.5,

  });

  if (!shader) return null;



  const pl = v.pointLights;

  if (

    !isString(pl.color) ||

    !isNumber(pl.flashIntensity) ||

    !isNumber(pl.decayLerp) ||

    !isNumber(pl.distance) ||

    !isNumber(pl.decay)

  ) {

    return null;

  }



  return {

    fireInterval: v.fireInterval,

    boltSpeed: v.boltSpeed,

    boltLifetime: v.boltLifetime,

    maxBolts: v.maxBolts,

    boltSize: [v.boltSize[0], v.boltSize[1]],

    colors: { core, glow },

    shader,

    pointLights: {

      color: pl.color,

      flashIntensity: pl.flashIntensity,

      decayLerp: pl.decayLerp,

      distance: pl.distance,

      decay: pl.decay,

    },

  };

}



function validateBlasterMuzzleSlot(v: unknown, legacy?: LegacyBlasterShared): BlasterMuzzleSlot | null {

  if (!isObject(v) || !isString(v.id) || !v.id.trim()) return null;

  const localPosition = validateVec3(v.localPosition);

  if (!localPosition) return null;



  const hasOwnWeapon =

    isNumber(v.fireInterval) &&

    isNumber(v.boltSpeed) &&

    isNumber(v.boltLifetime) &&

    isNumber(v.maxBolts) &&

    Array.isArray(v.boltSize) &&

    isObject(v.colors) &&

    isObject(v.shader) &&

    isObject(v.pointLights);



  let shared: LegacyBlasterShared | null = null;

  if (hasOwnWeapon) {

    shared = readLegacyBlasterShared(v);

  } else if (legacy) {

    shared = legacy;

  }

  if (!shared) return null;



  const slot: BlasterMuzzleSlot = {

    id: v.id,

    localPosition,

    ...shared,

  };

  if (v.lightPosition !== undefined) {

    const lightPosition = validateVec3(v.lightPosition);

    if (!lightPosition) return null;

    slot.lightPosition = lightPosition;

  }

  return slot;

}



function migrateLegacyBlasterMuzzles(v: Record<string, unknown>): BlasterMuzzleSlot[] | null {

  if (!isObject(v.muzzles)) return null;

  const left = validateVec3(v.muzzles.left);

  const right = validateVec3(v.muzzles.right);

  if (!left || !right) return null;

  const legacy = readLegacyBlasterShared(v);

  if (!legacy) return null;

  const pl = isObject(v.pointLights) ? v.pointLights : null;

  const plLeft = pl ? validateVec3(pl.left) : null;

  const plRight = pl ? validateVec3(pl.right) : null;

  const leftSlot = validateBlasterMuzzleSlot(

    { id: 'muzzle-left', localPosition: left, lightPosition: plLeft, ...legacy },

    legacy

  );

  const rightSlot = validateBlasterMuzzleSlot(

    { id: 'muzzle-right', localPosition: right, lightPosition: plRight, ...legacy },

    legacy

  );

  if (!leftSlot || !rightSlot) return null;

  return [leftSlot, rightSlot];

}



function validateBlaster(v: unknown): BlasterConfig | null {

  if (!isObject(v)) return null;



  const legacyShared = readLegacyBlasterShared(v);

  let muzzles: BlasterMuzzleSlot[] | null = null;



  if (Array.isArray(v.muzzles)) {

    if (v.muzzles.length === 0) return null;

    muzzles = [];

    for (const item of v.muzzles) {

      const slot = validateBlasterMuzzleSlot(item, legacyShared ?? undefined);

      if (!slot) return null;

      muzzles.push(slot);

    }

  } else {

    muzzles = migrateLegacyBlasterMuzzles(v);

  }



  if (!muzzles) return null;

  return { muzzles };

}



export type ValidateTurretResult =

  | { ok: true; data: TurretDefinition }

  | { ok: false; errors: string[] };



export function validateTurretDefinition(raw: unknown): ValidateTurretResult {

  const errors: string[] = [];

  if (!isObject(raw)) {

    return { ok: false, errors: ['Root must be an object'] };

  }



  if (raw.schemaVersion !== SCHEMA_VERSION) {

    errors.push(`schemaVersion must be ${SCHEMA_VERSION}`);

  }

  if (!isString(raw.id) || !raw.id.trim()) errors.push('id is required');

  if (!isString(raw.name)) errors.push('name is required');

  if (!KINDS.includes(raw.kind as TurretKind)) errors.push('kind must be beam or blaster');



  const kind = raw.kind as TurretKind;

  const visual = validateVisual(raw.visual);

  const pivot = validatePivot(raw.pivot);

  const rotationOrigin =

    raw.rotationOrigin === undefined

      ? DEFAULT_ROTATION_ORIGIN

      : validateRotationOrigin(raw.rotationOrigin);

  const rotation = validateRotation(raw.rotation);

  if (!visual) errors.push('visual invalid');

  if (!pivot) errors.push('pivot invalid');

  if (!rotationOrigin) errors.push('rotationOrigin invalid');

  if (!rotation) errors.push('rotation invalid');

  else {

    const span = rotation.maxAngle - rotation.minAngle;

    if (span > Math.PI * 2 + 1e-6) {

      errors.push('rotation arc must be at most 360 degrees');

    }

  }



  let beam: BeamConfig | undefined;

  let blaster: BlasterConfig | undefined;



  if (kind === 'beam') {

    if (raw.beam === undefined) errors.push('beam config required for kind beam');

    else {

      const b = validateBeam(raw.beam);

      if (!b) errors.push('beam config invalid');

      else beam = b;

    }

    if (raw.blaster !== undefined) errors.push('blaster must be omitted for kind beam');

  } else {

    if (raw.blaster === undefined) errors.push('blaster config required for kind blaster');

    else {

      const b = validateBlaster(raw.blaster);

      if (!b) errors.push('blaster config invalid');

      else blaster = b;

    }

    if (raw.beam !== undefined) errors.push('beam must be omitted for kind blaster');

  }



  if (errors.length > 0) return { ok: false, errors };



  return {

    ok: true,

    data: {

      schemaVersion: SCHEMA_VERSION,

      id: raw.id as string,

      name: raw.name as string,

      kind,

      visual: visual!,

      pivot: pivot!,

      rotationOrigin: rotationOrigin!,

      rotation: rotation!,

      beam,

      blaster,

    },

  };

}


