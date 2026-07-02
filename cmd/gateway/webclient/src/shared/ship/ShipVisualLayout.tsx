import type { ReactNode } from 'react';
import type { ShipDefinition, ShipEngineSlot } from '@/src/shared/types/ship';
import type { ShipWeaponSlot } from '@/src/shared/types/weapon';
import type { TurretDefinition } from '@/src/shared/types/turret';
import type { WorldTransform } from '@/src/shared/engine/simulator';
import type { InputMap } from '@/src/shared/engine/resolveMode';
import type { EngineModeId } from '@/src/shared/types/engine';
import { EngineInstances } from '@/src/shared/engine/EngineInstances';
import { ShipHullMesh, type HullPreviewTextureUrls } from '@/src/shared/ship/ShipHullMesh';
import { ShipWeaponMount } from '@/src/shared/ship/ShipWeaponMount';
import type { FiringInput } from '@/src/shared/weapon/resolveFiring';

export interface ShipWeaponRenderItem {
  slot: ShipWeaponSlot;
  definition: TurretDefinition;
  getAimAngle?: () => number;
  aimAngle?: number;
  firing?: FiringInput;
}

export interface ShipVisualLayoutProps {
  hull: ShipDefinition['hull'];
  engineSlots: ShipEngineSlot[];
  transform: WorldTransform;
  inputs: InputMap;
  weapons: ShipWeaponRenderItem[];
  previewMode?: EngineModeId;
  hullPreviewUrls?: HullPreviewTextureUrls;
  /** Extra nodes after hull (hit effects, etc.). */
  hullOverlay?: ReactNode;
  /** Editor gizmos rendered after weapons. */
  children?: ReactNode;
}

/**
 * Shared draw order for ship JSON (matches ship editor preview):
 * 1. Hull  2. Engines  3. Weapons  4. optional gizmos / shield
 */
export function ShipVisualLayout({
  hull,
  engineSlots,
  transform,
  inputs,
  weapons,
  previewMode,
  hullPreviewUrls,
  hullOverlay,
  children,
}: ShipVisualLayoutProps) {
  return (
    <>
      <ShipHullMesh hull={hull} previewUrls={hullPreviewUrls} />
      {hullOverlay}
      <EngineInstances
        slots={engineSlots}
        transform={transform}
        inputs={inputs}
        previewMode={previewMode}
      />
      {weapons.map((w) => (
        <ShipWeaponMount
          key={w.slot.id}
          slot={w.slot}
          definition={w.definition}
          getAimAngle={w.getAimAngle}
          aimAngle={w.aimAngle}
          firing={w.firing}
        />
      ))}
      {children}
    </>
  );
}
