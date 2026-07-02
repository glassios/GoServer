// High-fidelity battle ship: the SpaceShip2d hull/engine/turret stack driven by interpolated
// server snapshots, with a team-coloured ground ring. Pose/engine come from the store; weapons
// (Slice 4) and per-instance shield/heat (Slice 5) build on this.
import { useFrame } from '@react-three/fiber';
import { useRef, useState } from 'react';
import * as THREE from 'three';
import type { ShipEntity } from '@/src/game/entities/ShipEntity';
import type { GameInputState } from '@/src/game/input/gameInput';
import { EngineInstances } from '@/src/shared/engine/EngineInstances';
import type { WorldTransform } from '@/src/shared/engine/simulator';
import { ShipWeaponMount } from '@/src/shared/ship/ShipWeaponMount';
import { PlayerCombatHull } from '@/src/game/presentation/PlayerCombatHull';
import { ShipHeatProvider } from '@/src/game/presentation/ShipHeatContext';
import { THRUST_SPEED_THRESHOLD } from '@/src/net/coords';
import { store } from '@/src/net/session';
import { teamColor } from '@/src/net/teamColor';

const SHIP_LOCAL_TRANSFORM: WorldTransform = { x: 0, y: 0, z: 0, rotation: 0 };
const NO_INPUT: GameInputState = { w: false, a: false, s: false, d: false, fire: false, blaster: false };
const NO_FIRING = () => false;
const NO_AIM = () => 0;

export function BattleShipView({ entity, id }: { entity: ShipEntity; id: string }) {
  const groupRef = useRef<THREE.Group>(null);
  const [thrusting, setThrusting] = useState(false);
  const thrustingRef = useRef(false);

  const ent = store.entities.get(id);
  const color = ent ? teamColor(ent.state.faction_id) : '#ffffff';
  const hull = entity.definition.hull;
  const ringR = Math.max(hull.size[0], hull.size[1]) * 0.62;

  useFrame(() => {
    const g = groupRef.current;
    if (!g) return;
    const pose = store.sampleRender(id, performance.now());
    if (!pose) {
      g.visible = false;
      return;
    }
    g.visible = true;
    g.position.set(pose.x, pose.y, 0);
    g.rotation.z = pose.rotation;
    const m = pose.speed > THRUST_SPEED_THRESHOLD;
    if (m !== thrustingRef.current) {
      thrustingRef.current = m;
      setThrusting(m);
    }
  });

  return (
    <group ref={groupRef}>
      {/* team ring sits just behind the hull */}
      <mesh position={[0, 0, -0.3]}>
        <ringGeometry args={[ringR, ringR * 1.18, 32]} />
        <meshBasicMaterial color={color} transparent opacity={0.55} side={THREE.DoubleSide} />
      </mesh>
      <ShipHeatProvider shipGroupRef={groupRef} hullSize={hull.size}>
        <PlayerCombatHull hull={hull} shipGroupRef={groupRef} />
        <EngineInstances
          slots={entity.engineSlots}
          transform={SHIP_LOCAL_TRANSFORM}
          inputs={NO_INPUT}
          previewMode={thrusting ? 'thrust' : 'idle'}
        />
        {entity.turretMounts.map(
          (mount) =>
            mount.definition && (
              <ShipWeaponMount
                key={mount.slot.id}
                slot={mount.slot}
                definition={mount.definition}
                getAimAngle={NO_AIM}
                firing={NO_FIRING}
                combatHeat
              />
            )
        )}
      </ShipHeatProvider>
    </group>
  );
}
