import type { SchemaVersion } from './schema';
import type { EngineDefinition } from './engine';
import type { ShipWeaponSlot } from './weapon';

export interface ShipHullDefinition {
  size: [number, number];
  texture?: string;
  normalMap?: string;
  roughness?: number;
  metalness?: number;
}

export interface ShipEngineSlot {
  id: string;
  engineAsset: string;
  mountOverride?: Partial<EngineDefinition['mount']>;
  inputBindings?: {
    thrust?: string[];
    reverse?: string[];
    turnAssistLeft?: string[];
    turnAssistRight?: string[];
  };
}

export interface ShipDefinition {
  schemaVersion: SchemaVersion;
  id: string;
  name: string;
  hull: ShipHullDefinition;
  engines: ShipEngineSlot[];
  weapons: ShipWeaponSlot[];
}
