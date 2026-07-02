// Battle instance view: every battle ship rendered at full SpaceShip2d fidelity, framed by an
// observer auto-fit camera. Ships are AI-controlled; the player observes and sets tactics (HUD,
// Slice 7). Weapons (Slice 4), shields/heat (Slice 5) and explosions (Slice 6) build on this.
import { useFrame, useThree } from '@react-three/fiber';
import { useEffect, useState } from 'react';
import * as THREE from 'three';
import { gameAssetRegistry } from '@/src/game/assets/GameAssetRegistry';
import type { ShipEntity } from '@/src/game/entities/ShipEntity';
import { store } from '@/src/net/session';
import { useEntityIds } from '@/src/net/hooks';
import { shipPrefabFor } from '@/src/net/visualMapping';
import { isShipEntity } from '@/src/net/types';
import { BattleShipView } from './BattleShipView';

/** Loads the prefab for one battle ship (by ship_type) then renders it at full fidelity. */
function BattleShip({ id }: { id: string }) {
  const [entity, setEntity] = useState<ShipEntity | null>(null);

  useEffect(() => {
    let cancelled = false;
    const ent = store.entities.get(id);
    const asset = shipPrefabFor(ent?.shipType ?? '');
    void gameAssetRegistry
      .createShipEntity(asset)
      .then((e) => {
        if (!cancelled) setEntity(e);
      })
      .catch((err) => console.error('Failed to load battle ship', id, err));
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (!entity) return null;
  return <BattleShipView entity={entity} id={id} />;
}

/** Observer camera: smoothly frames the bounding box of all battle ships at any scale. */
function ObserverCamera({ ids }: { ids: string[] }) {
  const { size } = useThree();
  useFrame((state, delta) => {
    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;
    let count = 0;
    for (const id of ids) {
      const pose = store.sampleRender(id, performance.now());
      if (!pose) continue;
      minX = Math.min(minX, pose.x);
      minY = Math.min(minY, pose.y);
      maxX = Math.max(maxX, pose.x);
      maxY = Math.max(maxY, pose.y);
      count++;
    }
    if (count === 0) return;

    const cx = (minX + maxX) / 2;
    const cy = (minY + maxY) / 2;
    const w = Math.max(maxX - minX, 8);
    const h = Math.max(maxY - minY, 8);
    const margin = 1.6;
    const targetZoom = THREE.MathUtils.clamp(
      Math.min(size.width / (w * margin), size.height / (h * margin)),
      6,
      90
    );

    const cam = state.camera as THREE.OrthographicCamera;
    const k = 1 - Math.exp(-3 * delta);
    cam.position.x = THREE.MathUtils.lerp(cam.position.x, cx, k);
    cam.position.y = THREE.MathUtils.lerp(cam.position.y, cy, k);
    const nextZoom = THREE.MathUtils.lerp(cam.zoom, targetZoom, k);
    if (Math.abs(nextZoom - cam.zoom) > 0.001) {
      cam.zoom = nextZoom;
      cam.updateProjectionMatrix();
    }
  });
  return null;
}

export function BattleScene() {
  const ids = useEntityIds();
  const shipIds = ids.filter((id) => {
    const e = store.entities.get(id);
    return e && isShipEntity(e.entityType);
  });

  return (
    <>
      <ObserverCamera ids={shipIds} />
      {shipIds.map((id) => (
        <BattleShip key={id} id={id} />
      ))}
    </>
  );
}
