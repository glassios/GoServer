import { Line } from '@react-three/drei';
import { useFrame } from '@react-three/fiber';
import { useMemo, useRef } from 'react';
import * as THREE from 'three';
import type { TurretDefinition, TurretVisual } from '@/src/shared/types/turret';
import type { ShipWeaponSlot } from '@/src/shared/types/weapon';
import { TurretVisualMesh } from '@/src/shared/turret/TurretVisualMesh';
import { BeamWeapon } from '@/src/shared/weapon/BeamWeapon';
import { BlasterWeaponPreview } from '@/src/shared/weapon/BlasterWeaponPreview';
import type { FiringInput } from '@/src/shared/weapon/resolveFiring';
import { useCombatHeatMaterial } from '@/src/game/presentation/ShipHeatContext';

export interface ShipWeaponMountProps {
  slot: ShipWeaponSlot;
  definition: TurretDefinition;
  getAimAngle?: () => number;
  aimAngle?: number;
  firing?: FiringInput;
  showEditorGizmos?: boolean;
  /** Game combat: heat shader + propagate hits to all overlapping meshes. */
  combatHeat?: boolean;
}

function TurretArcLimits({ minAngle, maxAngle }: { minAngle: number; maxAngle: number }) {
  const radius = 1.8;
  const minPoints = useMemo(
    (): [number, number, number][] => [
      [0, 0, 0.01],
      [Math.cos(minAngle) * radius, Math.sin(minAngle) * radius, 0.01],
    ],
    [minAngle]
  );
  const maxPoints = useMemo(
    (): [number, number, number][] => [
      [0, 0, 0.01],
      [Math.cos(maxAngle) * radius, Math.sin(maxAngle) * radius, 0.01],
    ],
    [maxAngle]
  );
  return (
    <>
      <Line points={minPoints} color="#f59e0b" transparent opacity={0.65} lineWidth={1} />
      <Line points={maxPoints} color="#f59e0b" transparent opacity={0.65} lineWidth={1} />
    </>
  );
}

function MountAxisLine({ rotation }: { rotation: number }) {
  const len = 1.4;
  const points = useMemo(
    (): [number, number, number][] => [
      [0, 0, 0.02],
      [Math.cos(rotation) * len, Math.sin(rotation) * len, 0.02],
    ],
    [rotation]
  );
  return <Line points={points} color="#e879f9" transparent opacity={0.85} lineWidth={1.5} />;
}

function CombatTurretVisual({ visual }: { visual: TurretVisual }) {
  const { onBeforeCompile, handlePointerDown } = useCombatHeatMaterial();

  return (
    <TurretVisualMesh
      visual={visual}
      onBeforeCompile={onBeforeCompile}
      onPointerDown={handlePointerDown}
    />
  );
}

/**
 * Single weapon slot — same hierarchy as ship editor preview.
 */
export function ShipWeaponMount({
  slot,
  definition,
  getAimAngle,
  aimAngle = 0,
  firing = false,
  showEditorGizmos = false,
  combatHeat = false,
}: ShipWeaponMountProps) {
  const aimGroupRef = useRef<THREE.Group>(null);
  const { rotationOrigin, rotation, kind, beam, blaster, visual } = definition;
  const scale = slot.scale > 0 ? slot.scale : 1;
  const { localPosition, rotation: mountRotation } = slot.mount;

  useFrame(() => {
    if (!aimGroupRef.current) return;
    const aim = getAimAngle ? getAimAngle() : aimAngle;
    aimGroupRef.current.rotation.z = mountRotation + aim;
  });

  return (
    <group
      position={[localPosition.x, localPosition.y, localPosition.z]}
      scale={[scale, scale, scale]}
    >
      {showEditorGizmos && (
        <>
          <MountAxisLine rotation={mountRotation} />
          <TurretArcLimits minAngle={rotation.minAngle} maxAngle={rotation.maxAngle} />
        </>
      )}
      <group ref={aimGroupRef}>
        <group position={[-rotationOrigin.x, -rotationOrigin.y, -rotationOrigin.z]}>
          {combatHeat ? (
            <CombatTurretVisual visual={visual} />
          ) : (
            <TurretVisualMesh visual={visual} />
          )}
          {kind === 'beam' && beam && <BeamWeapon config={beam} firing={firing} />}
          {kind === 'blaster' && blaster && (
            <BlasterWeaponPreview config={blaster} firing={firing} />
          )}
        </group>
      </group>
    </group>
  );
}
