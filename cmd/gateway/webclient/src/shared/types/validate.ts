import { SCHEMA_VERSION } from './schema';
import {
  ENGINE_MODE_IDS,
  type EngineDefinition,
  type EngineModeConfig,
  type EngineModeId,
  type EngineSide,
  type FlameColorStop,
} from './engine';
import type { ShipDefinition, ShipEngineSlot, ShipHullDefinition } from './ship';
import type { ShipWeaponSlot } from './weapon';

const SIDES: EngineSide[] = ['left', 'right', 'center'];

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function isNumber(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v);
}

function isString(v: unknown): v is string {
  return typeof v === 'string';
}

function validateModeConfig(v: unknown, path: string): EngineModeConfig | null {
  if (!isObject(v)) return null;
  const fields: (keyof EngineModeConfig)[] = [
    'emitRate',
    'poolWeight',
    'lifeMin',
    'lifeMax',
    'speedBase',
    'speedVar',
    'lateralVelocity',
  ];
  const out = {} as EngineModeConfig;
  for (const f of fields) {
    if (!isNumber(v[f])) return null;
    out[f] = v[f] as number;
  }
  return out;
}

function validateStops(v: unknown): FlameColorStop[] | null {
  if (!Array.isArray(v) || v.length < 2) return null;
  const stops: FlameColorStop[] = [];
  for (const item of v) {
    if (!isObject(item) || !isNumber(item.position)) return null;
    const rgba = item.rgba;
    if (!Array.isArray(rgba) || rgba.length !== 4 || !rgba.every(isNumber)) return null;
    stops.push({
      position: Math.max(0, Math.min(1, item.position)),
      rgba: [rgba[0], rgba[1], rgba[2], rgba[3]] as [number, number, number, number],
    });
  }
  return stops;
}

export type ValidateResult =
  | { ok: true; data: EngineDefinition }
  | { ok: false; errors: string[] };

export function validateEngineDefinition(raw: unknown): ValidateResult {
  const errors: string[] = [];
  if (!isObject(raw)) {
    return { ok: false, errors: ['Root must be an object'] };
  }

  if (raw.schemaVersion !== SCHEMA_VERSION) {
    errors.push(`schemaVersion must be ${SCHEMA_VERSION}`);
  }
  if (!isString(raw.id) || !raw.id.trim()) errors.push('id is required');
  if (!isString(raw.name)) errors.push('name is required');

  if (!isObject(raw.mount)) {
    errors.push('mount is required');
  } else {
    if (!SIDES.includes(raw.mount.side as EngineSide)) errors.push('mount.side invalid');
    if (!isObject(raw.mount.localPosition)) errors.push('mount.localPosition required');
    else {
      for (const k of ['x', 'y', 'z'] as const) {
        if (!isNumber(raw.mount.localPosition[k])) errors.push(`mount.localPosition.${k} invalid`);
      }
    }
    if (!isNumber(raw.mount.lateralOffset)) errors.push('mount.lateralOffset invalid');
    if (!isNumber(raw.mount.exhaustAngle)) errors.push('mount.exhaustAngle invalid');
  }

  if (!isObject(raw.flame) || !validateStops(raw.flame.stops)) {
    errors.push('flame.stops invalid (need 2+ stops with position and rgba[4])');
  }

  if (!isObject(raw.particle)) {
    errors.push('particle is required');
  } else {
    for (const k of [
      'maxCount',
      'quadSize',
      'opacity',
      'backOffset',
      'zOffset',
      'positionSpread',
    ] as const) {
      if (!isNumber(raw.particle[k])) errors.push(`particle.${k} invalid`);
    }
  }

  if (!isObject(raw.modes)) {
    errors.push('modes is required');
  } else {
    for (const modeId of ENGINE_MODE_IDS) {
      if (!validateModeConfig(raw.modes[modeId], modeId)) {
        errors.push(`modes.${modeId} invalid`);
      }
    }
  }

  if (!isObject(raw.pointLight)) {
    errors.push('pointLight is required');
  } else {
    if (typeof raw.pointLight.enabled !== 'boolean') errors.push('pointLight.enabled invalid');
    if (!isString(raw.pointLight.color)) errors.push('pointLight.color invalid');
    if (!isObject(raw.pointLight.position)) errors.push('pointLight.position invalid');
    for (const k of ['distance', 'decay', 'intensityMin', 'intensityMax', 'rampUp', 'rampDown'] as const) {
      if (!isNumber(raw.pointLight[k])) errors.push(`pointLight.${k} invalid`);
    }
  }

  if (errors.length > 0) return { ok: false, errors };

  const mount = raw.mount as Record<string, unknown>;
  const mountPos = mount.localPosition as Record<string, number>;
  const flame = raw.flame as Record<string, unknown>;
  const particle = raw.particle as Record<string, number>;
  const pointLight = raw.pointLight as Record<string, unknown>;
  const plPos = pointLight.position as Record<string, number>;
  const modesRaw = raw.modes as Record<string, unknown>;

  const modes = {} as Record<EngineModeId, EngineModeConfig>;
  for (const modeId of ENGINE_MODE_IDS) {
    modes[modeId] = validateModeConfig(modesRaw[modeId], modeId)!;
  }

  const data: EngineDefinition = {
    schemaVersion: SCHEMA_VERSION,
    id: raw.id as string,
    name: raw.name as string,
    mount: {
      side: mount.side as EngineSide,
      localPosition: { x: mountPos.x, y: mountPos.y, z: mountPos.z },
      lateralOffset: mount.lateralOffset as number,
      exhaustAngle: mount.exhaustAngle as number,
    },
    flame: {
      stops: validateStops(flame.stops)!,
      canvasSize: isNumber(flame.canvasSize) ? (flame.canvasSize as number) : undefined,
    },
    particle: {
      maxCount: particle.maxCount,
      quadSize: particle.quadSize,
      opacity: particle.opacity,
      backOffset: particle.backOffset,
      zOffset: particle.zOffset,
      positionSpread: particle.positionSpread,
    },
    modes,
    pointLight: {
      enabled: pointLight.enabled as boolean,
      color: pointLight.color as string,
      position: { x: plPos.x, y: plPos.y, z: plPos.z },
      distance: pointLight.distance as number,
      decay: pointLight.decay as number,
      intensityMin: pointLight.intensityMin as number,
      intensityMax: pointLight.intensityMax as number,
      rampUp: pointLight.rampUp as number,
      rampDown: pointLight.rampDown as number,
    },
  };

  return { ok: true, data };
}

export type ValidateShipResult =
  | { ok: true; data: ShipDefinition }
  | { ok: false; errors: string[] };

function isStringArray(v: unknown): v is string[] {
  return Array.isArray(v) && v.every(isString);
}

function validateMountOverride(v: unknown): Partial<EngineDefinition['mount']> | undefined {
  if (v === undefined) return undefined;
  if (!isObject(v)) return undefined;
  const out: Partial<EngineDefinition['mount']> = {};
  if (v.side !== undefined) {
    if (!SIDES.includes(v.side as EngineSide)) return undefined;
    out.side = v.side as EngineSide;
  }
  if (v.lateralOffset !== undefined) {
    if (!isNumber(v.lateralOffset)) return undefined;
    out.lateralOffset = v.lateralOffset;
  }
  if (v.exhaustAngle !== undefined) {
    if (!isNumber(v.exhaustAngle)) return undefined;
    out.exhaustAngle = v.exhaustAngle;
  }
  if (v.localPosition !== undefined) {
    if (!isObject(v.localPosition)) return undefined;
    const lp = v.localPosition as Record<string, unknown>;
    if (!isNumber(lp.x) || !isNumber(lp.y) || !isNumber(lp.z)) return undefined;
    out.localPosition = { x: lp.x, y: lp.y, z: lp.z };
  }
  return out;
}

function validateEngineSlot(v: unknown, index: number): ShipEngineSlot | null {
  if (!isObject(v)) return null;
  if (!isString(v.id) || !v.id.trim()) return null;
  if (!isString(v.engineAsset) || !v.engineAsset.trim()) return null;
  const mountOverride = validateMountOverride(v.mountOverride);
  if (v.mountOverride !== undefined && mountOverride === undefined) return null;

  const slot: ShipEngineSlot = {
    id: v.id as string,
    engineAsset: v.engineAsset as string,
  };
  if (mountOverride) slot.mountOverride = mountOverride;

  if (v.inputBindings !== undefined) {
    if (!isObject(v.inputBindings)) return null;
    const ib = v.inputBindings as Record<string, unknown>;
    const bindings: ShipEngineSlot['inputBindings'] = {};
    if (ib.thrust !== undefined) {
      if (!isStringArray(ib.thrust)) return null;
      bindings.thrust = ib.thrust;
    }
    if (ib.turnAssistLeft !== undefined) {
      if (!isStringArray(ib.turnAssistLeft)) return null;
      bindings.turnAssistLeft = ib.turnAssistLeft;
    }
    if (ib.turnAssistRight !== undefined) {
      if (!isStringArray(ib.turnAssistRight)) return null;
      bindings.turnAssistRight = ib.turnAssistRight;
    }
    if (ib.reverse !== undefined) {
      if (!isStringArray(ib.reverse)) return null;
      bindings.reverse = ib.reverse;
    }
    if (Object.keys(bindings).length > 0) slot.inputBindings = bindings;
  }

  void index;
  return slot;
}

function validateWeaponSlot(v: unknown): ShipWeaponSlot | null {
  if (!isObject(v)) return null;
  if (!isString(v.id) || !v.id.trim()) return null;
  const turretAsset = isString(v.turretAsset)
    ? v.turretAsset
    : isString(v.weaponAsset)
      ? (v.weaponAsset as string)
      : null;
  if (!turretAsset) return null;
  if (!isObject(v.mount)) return null;
  const m = v.mount as Record<string, unknown>;
  if (!isObject(m.localPosition)) return null;
  const lp = m.localPosition as Record<string, unknown>;
  if (!isNumber(lp.x) || !isNumber(lp.y) || !isNumber(lp.z)) return null;
  if (!isNumber(m.rotation)) return null;
  const scale = v.scale === undefined ? 1 : isNumber(v.scale) && v.scale > 0 ? v.scale : null;
  if (scale === null) return null;
  return {
    id: v.id as string,
    turretAsset,
    scale,
    mount: {
      localPosition: { x: lp.x, y: lp.y, z: lp.z },
      rotation: m.rotation as number,
    },
  };
}

function validateHull(v: unknown): ShipHullDefinition | null {
  if (!isObject(v)) return null;
  if (!Array.isArray(v.size) || v.size.length !== 2 || !v.size.every(isNumber)) return null;
  const hull: ShipHullDefinition = {
    size: [v.size[0] as number, v.size[1] as number],
  };
  if (v.texture !== undefined) {
    if (!isString(v.texture)) return null;
    hull.texture = v.texture;
  }
  if (v.normalMap !== undefined) {
    if (!isString(v.normalMap)) return null;
    hull.normalMap = v.normalMap;
  }
  if (v.roughness !== undefined) {
    if (!isNumber(v.roughness)) return null;
    hull.roughness = v.roughness;
  }
  if (v.metalness !== undefined) {
    if (!isNumber(v.metalness)) return null;
    hull.metalness = v.metalness;
  }
  return hull;
}

export function validateShipDefinition(raw: unknown): ValidateShipResult {
  const errors: string[] = [];
  if (!isObject(raw)) {
    return { ok: false, errors: ['Root must be an object'] };
  }

  if (raw.schemaVersion !== SCHEMA_VERSION) {
    errors.push(`schemaVersion must be ${SCHEMA_VERSION}`);
  }
  if (!isString(raw.id) || !raw.id.trim()) errors.push('id is required');
  if (!isString(raw.name)) errors.push('name is required');

  const hull = validateHull(raw.hull);
  if (!hull) errors.push('hull invalid (size: [number, number] required)');

  if (!Array.isArray(raw.engines)) {
    errors.push('engines must be an array');
  }

  const engines: ShipEngineSlot[] = [];
  if (Array.isArray(raw.engines)) {
    const seen = new Set<string>();
    raw.engines.forEach((item, i) => {
      const slot = validateEngineSlot(item, i);
      if (!slot) {
        errors.push(`engines[${i}] invalid`);
        return;
      }
      if (seen.has(slot.id)) errors.push(`engines[${i}]: duplicate id "${slot.id}"`);
      seen.add(slot.id);
      engines.push(slot);
    });
  }

  const weapons: ShipWeaponSlot[] = [];
  if (raw.weapons === undefined) {
    errors.push('weapons array is required (use [])');
  } else if (!Array.isArray(raw.weapons)) {
    errors.push('weapons must be an array');
  } else {
    raw.weapons.forEach((item, i) => {
      const slot = validateWeaponSlot(item);
      if (!slot) errors.push(`weapons[${i}] invalid`);
      else weapons.push(slot);
    });
  }

  if (errors.length > 0) return { ok: false, errors };

  const data: ShipDefinition = {
    schemaVersion: SCHEMA_VERSION,
    id: raw.id as string,
    name: raw.name as string,
    hull: hull!,
    engines,
    weapons,
  };

  return { ok: true, data };
}
