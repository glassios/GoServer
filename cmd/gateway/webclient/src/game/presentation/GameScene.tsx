import { useFrame } from '@react-three/fiber';
import { useEffect, useState } from 'react';
import * as THREE from 'three';
import { gameAssetRegistry } from '@/src/game/assets/GameAssetRegistry';
import { PLAYER_SHIP_ASSET } from '@/src/game/config/gameConfig';
import type { ShipEntity } from '@/src/game/entities/ShipEntity';
import { gameInput } from '@/src/game/input/gameInput';
import { PlayerShipView } from '@/src/game/presentation/PlayerShipView';
import { syncGlobalShipState } from '@/src/game/sync/globalShipState';

export function GameScene() {
  const [player, setPlayer] = useState<ShipEntity | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void gameAssetRegistry
      .createShipEntity(PLAYER_SHIP_ASSET)
      .then((entity) => {
        if (!cancelled) setPlayer(entity);
      })
      .catch((err) => {
        console.error('Failed to load player ship:', err);
        if (!cancelled) setLoadError(String(err));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useFrame((state, delta) => {
    if (!player) return;

    player.tickMovement(delta, gameInput);
    syncGlobalShipState(player);

    const targetX = player.transform.x;
    const targetY = player.transform.y;
    state.camera.position.x = THREE.MathUtils.lerp(state.camera.position.x, targetX, 5 * delta);
    state.camera.position.y = THREE.MathUtils.lerp(state.camera.position.y, targetY, 5 * delta);
  });

  if (loadError) {
    return (
      <mesh>
        <planeGeometry args={[4, 1]} />
        <meshBasicMaterial color="#ef4444" />
      </mesh>
    );
  }

  if (!player) {
    return (
      <mesh>
        <sphereGeometry args={[0.5, 12, 12]} />
        <meshBasicMaterial color="#64748b" wireframe />
      </mesh>
    );
  }

  return <PlayerShipView entity={player} />;
}
