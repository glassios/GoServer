import { useFrame } from '@react-three/fiber';
import { useRef } from 'react';
import * as THREE from 'three';
import type { ShipEntity } from '@/src/game/entities/ShipEntity';
import { gameInput } from '@/src/game/input/gameInput';
import { EngineInstances } from '@/src/shared/engine/EngineInstances';
import type { WorldTransform } from '@/src/shared/engine/simulator';
import { ShipWeaponMount } from '@/src/shared/ship/ShipWeaponMount';
import { PlayerCombatHull } from '@/src/game/presentation/PlayerCombatHull';
import { PlayerShield } from '@/src/game/presentation/PlayerShield';
import { ShipHeatProvider } from '@/src/game/presentation/ShipHeatContext';

/** Engines simulate in ship-local space; the parent group applies world pose. */
const SHIP_LOCAL_TRANSFORM: WorldTransform = { x: 0, y: 0, z: 0, rotation: 0 };

function readWeaponFiring(): boolean {
  return gameInput.fire || gameInput.blaster;
}

interface PlayerShipViewProps {
  entity: ShipEntity;
}

/** Draw order matches ship editor: hull → engines → weapons → shield */
export function PlayerShipView({ entity }: PlayerShipViewProps) {
  const groupRef = useRef<THREE.Group>(null);

  useFrame(() => {
    if (!groupRef.current) return;
    const t = entity.transform;
    groupRef.current.position.set(t.x, t.y, t.z);
    groupRef.current.rotation.z = t.rotation;
    entity.setWeaponFiring(readWeaponFiring());
  });

  return (
    <group ref={groupRef}>
      <ShipHeatProvider shipGroupRef={groupRef} hullSize={entity.definition.hull.size}>
        <PlayerCombatHull hull={entity.definition.hull} shipGroupRef={groupRef} />
        <EngineInstances
          slots={entity.engineSlots}
          transform={SHIP_LOCAL_TRANSFORM}
          inputs={gameInput}
        />
        {entity.turretMounts.map(
          (mount) =>
            mount.definition && (
              <ShipWeaponMount
                key={mount.slot.id}
                slot={mount.slot}
                definition={mount.definition}
                getAimAngle={() => mount.aimAngle}
                firing={readWeaponFiring}
                combatHeat
              />
            )
        )}
      </ShipHeatProvider>
      <PlayerShield shipGroupRef={groupRef} />
    </group>
  );
}
