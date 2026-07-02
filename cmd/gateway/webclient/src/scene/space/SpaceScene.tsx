// Schematic open-world view: every entity is a cheap icon keyed by entity_type. The camera
// follows the local player; clicking a CombatMarker joins that battle (→ system_transition →
// BattleScene). High-fidelity rendering is reserved for the battle room.
import { ThreeEvent, useFrame } from '@react-three/fiber';
import { useMemo, useRef } from 'react';
import * as THREE from 'three';
import {
  ENTITY_TYPE_NPC,
  ENTITY_TYPE_PLAYER,
} from '@/src/net/types';
import { SPACE_COLORS, shipColor } from '@/src/net/spaceColors';
import { store } from '@/src/net/session';
import { connection } from '@/src/net/session';
import { useEntityIds } from '@/src/net/hooks';

// domain.EntityType
const T_ASTEROID = 2;
const T_STATION = 4;
const T_JUMPGATE = 5;
const T_LOOT = 6;
const T_COMBAT_MARKER = 7;
const T_SPACEBASE = 8;

/** Forward-pointing chevron (nose at +X), matching the legacy ship glyph. */
function makeShipShape(): THREE.Shape {
  const s = new THREE.Shape();
  s.moveTo(0.5, 0);
  s.lineTo(-0.35, -0.32);
  s.lineTo(-0.18, 0);
  s.lineTo(-0.35, 0.32);
  s.closePath();
  return s;
}

function ShipIcon({ id }: { id: string }) {
  const shape = useMemo(makeShipShape, []);
  const ent = store.entities.get(id);
  const isLocal = store.localPlayerId === id;
  const color = ent ? shipColor(ent.state, isLocal) : '#ffffff';
  const scale = isLocal ? 0.55 : 0.45;
  return (
    <mesh scale={scale}>
      <shapeGeometry args={[shape]} />
      <meshBasicMaterial color={color} />
    </mesh>
  );
}

function EntityIcon({ id, type, onJoin }: { id: string; type: number; onJoin: (id: string) => void }) {
  const pulseRef = useRef<THREE.Group>(null);

  // CombatMarker pulses and is clickable to join the battle.
  useFrame(({ clock }) => {
    if (pulseRef.current) {
      const p = 1 + Math.sin(clock.elapsedTime * 6) * 0.12;
      pulseRef.current.scale.setScalar(p);
    }
  });

  switch (type) {
    case ENTITY_TYPE_PLAYER:
    case ENTITY_TYPE_NPC:
      return <ShipIcon id={id} />;
    case T_ASTEROID:
      return (
        <mesh>
          <circleGeometry args={[0.6, 8]} />
          <meshBasicMaterial color={SPACE_COLORS.asteroid} />
        </mesh>
      );
    case T_STATION:
      return (
        <mesh>
          <circleGeometry args={[1.5, 6]} />
          <meshBasicMaterial color={SPACE_COLORS.station} wireframe />
        </mesh>
      );
    case T_JUMPGATE:
      return (
        <mesh>
          <ringGeometry args={[1.0, 1.3, 24]} />
          <meshBasicMaterial color={SPACE_COLORS.jumpGate} />
        </mesh>
      );
    case T_LOOT:
      return (
        <mesh>
          <planeGeometry args={[0.4, 0.4]} />
          <meshBasicMaterial color={SPACE_COLORS.loot} wireframe />
        </mesh>
      );
    case T_SPACEBASE:
      return (
        <mesh>
          <circleGeometry args={[1.3, 6]} />
          <meshBasicMaterial color={SPACE_COLORS.spaceBase} wireframe />
        </mesh>
      );
    case T_COMBAT_MARKER:
      return (
        <group
          ref={pulseRef}
          onClick={(e: ThreeEvent<MouseEvent>) => {
            e.stopPropagation();
            onJoin(id);
          }}
          onPointerOver={() => (document.body.style.cursor = 'pointer')}
          onPointerOut={() => (document.body.style.cursor = 'default')}
        >
          <mesh>
            <ringGeometry args={[0.9, 1.1, 24]} />
            <meshBasicMaterial color={SPACE_COLORS.combatMarker} />
          </mesh>
          {/* crossed swords */}
          <mesh rotation={[0, 0, Math.PI / 4]}>
            <planeGeometry args={[1.6, 0.14]} />
            <meshBasicMaterial color={SPACE_COLORS.combatMarker} />
          </mesh>
          <mesh rotation={[0, 0, -Math.PI / 4]}>
            <planeGeometry args={[1.6, 0.14]} />
            <meshBasicMaterial color={SPACE_COLORS.combatMarker} />
          </mesh>
        </group>
      );
    default:
      return null;
  }
}

function SpaceEntity({ id, onJoin }: { id: string; onJoin: (id: string) => void }) {
  const groupRef = useRef<THREE.Group>(null);
  const ent = store.entities.get(id);
  const type = ent?.entityType ?? -1;
  // Asteroids/stations don't spin with snapshot rotation; ships do.
  const orient = type === ENTITY_TYPE_PLAYER || type === ENTITY_TYPE_NPC;

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
    if (orient) g.rotation.z = pose.rotation;
  });

  if (type < 0) return null;
  return (
    <group ref={groupRef}>
      <EntityIcon id={id} type={type} onJoin={onJoin} />
    </group>
  );
}

export function SpaceScene() {
  const ids = useEntityIds();

  const handleJoin = (markerId: string) => {
    const ent = store.entities.get(markerId);
    if (!ent) return;
    // CombatMarker encodes the battle instance id in target_id (see network/snapshot.go).
    const instanceId = ent.state.target_id;
    if (!instanceId || instanceId === '0') return;
    if (window.confirm(`Вступить в бой (${ent.state.name || instanceId})?`)) {
      connection.sendJoinCombat(instanceId, 0); // FFA for now; side-select arrives with the HUD
    }
  };

  // Camera follows the interpolated local player.
  useFrame((state, delta) => {
    const id = store.localPlayerId;
    if (!id) return;
    const pose = store.sampleRender(id, performance.now());
    if (pose) {
      const k = 1 - Math.exp(-5 * delta);
      state.camera.position.x = THREE.MathUtils.lerp(state.camera.position.x, pose.x, k);
      state.camera.position.y = THREE.MathUtils.lerp(state.camera.position.y, pose.y, k);
    }
  });

  return (
    <>
      {ids.map((id) => (
        <SpaceEntity key={id} id={id} onJoin={handleJoin} />
      ))}
    </>
  );
}
