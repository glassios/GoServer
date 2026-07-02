import { useCallback, useEffect, useState } from 'react';
import type { ThreeEvent } from '@react-three/fiber';
import * as THREE from 'three';
import type { ShipHullDefinition } from '@/src/shared/types/ship';
import { ShipHullMesh } from '@/src/shared/ship/ShipHullMesh';
import { ExplosionSprite } from '@/src/game/presentation/ExplosionSprite';
import { getExplosionSheetLayout, loadExplosionSheetTexture } from '@/src/game/presentation/explosionTexture';
import { useCombatHeatMaterial } from '@/src/game/presentation/ShipHeatContext';

interface PlayerCombatHullProps {
  hull: ShipHullDefinition;
  shipGroupRef: React.RefObject<THREE.Group | null>;
}

export function PlayerCombatHull({ hull, shipGroupRef }: PlayerCombatHullProps) {
  const { onBeforeCompile, handlePointerDown } = useCombatHeatMaterial();

  const [explosions, setExplosions] = useState<
    { id: number; position: THREE.Vector3; time: number }[]
  >([]);

  useEffect(() => {
    void loadExplosionSheetTexture();
  }, []);

  const handlePointerDownWithExplosion = useCallback(
    (e: ThreeEvent<PointerEvent>) => {
      handlePointerDown(e);

      const pt = shipGroupRef.current
        ? shipGroupRef.current.worldToLocal(e.point.clone())
        : e.point;

      setExplosions((prev) => {
        const now = performance.now();
        const active = prev.filter((exp) => now - exp.time < getExplosionSheetLayout().durationMs);
        return [
          ...active,
          {
            id: Math.random(),
            position: new THREE.Vector3(pt.x, pt.y, pt.z + 0.2),
            time: now,
          },
        ].slice(-10);
      });
    },
    [handlePointerDown, shipGroupRef]
  );

  return (
    <>
      <ShipHullMesh
        hull={hull}
        onBeforeCompile={onBeforeCompile}
        onPointerDown={handlePointerDownWithExplosion}
      />
      {explosions.map((exp) => (
        <ExplosionSprite key={exp.id} position={exp.position} startTime={exp.time} />
      ))}
    </>
  );
}
