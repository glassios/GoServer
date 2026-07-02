import { useFrame } from '@react-three/fiber';
import { useEffect, useRef, useState } from 'react';
import * as THREE from 'three';
import {
  applyExplosionSheetFrame,
  createExplosionFrameTexture,
  getExplosionDisplaySize,
  getExplosionSheetLayout,
  getExplosionSheetTexture,
  loadExplosionSheetTexture,
} from '@/src/game/presentation/explosionTexture';

interface ExplosionSpriteProps {
  position: THREE.Vector3;
  startTime: number;
}

export function ExplosionSprite({ position, startTime }: ExplosionSpriteProps) {
  const materialRef = useRef<THREE.MeshBasicMaterial>(null);
  const meshRef = useRef<THREE.Mesh>(null);
  const textureRef = useRef<THREE.Texture | null>(null);
  const [ready, setReady] = useState(false);
  const [displaySize, setDisplaySize] = useState<[number, number]>([1.12, 1.12]);

  useEffect(() => {
    let cancelled = false;
    void loadExplosionSheetTexture().then((layout) => {
      if (cancelled) return;
      const source = getExplosionSheetTexture();
      if (!source) return;
      const tex = createExplosionFrameTexture(source, 0);
      textureRef.current = tex;
      if (materialRef.current) {
        materialRef.current.map = tex;
        materialRef.current.needsUpdate = true;
      }
      setDisplaySize(getExplosionDisplaySize(layout));
      setReady(true);
    });
    return () => {
      cancelled = true;
      textureRef.current?.dispose();
      textureRef.current = null;
    };
  }, []);

  useFrame(() => {
    const tex = textureRef.current;
    if (!tex || !materialRef.current || !meshRef.current || !ready) return;

    const age = performance.now() - startTime;
    const { durationMs, frameCount } = getExplosionSheetLayout();

    if (age >= durationMs) {
      meshRef.current.visible = false;
      return;
    }

    meshRef.current.visible = true;
    const frame = Math.min(frameCount - 1, Math.floor((age / durationMs) * frameCount));
    applyExplosionSheetFrame(tex, frame);
  });

  const [w, h] = displaySize;

  return (
    <mesh ref={meshRef} position={position} renderOrder={20}>
      <planeGeometry args={[w, h]} />
      <meshBasicMaterial
        ref={materialRef}
        transparent
        depthWrite={false}
        blending={THREE.AdditiveBlending}
      />
    </mesh>
  );
}
