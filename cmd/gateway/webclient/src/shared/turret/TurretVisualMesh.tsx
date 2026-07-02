import { useEffect, useState, type Ref } from 'react';
import * as THREE from 'three';
import type { TurretVisual } from '@/src/shared/types/turret';
import { assetUrl } from '@/src/shared/assetUrl';

export interface TurretPreviewTextureUrls {
  diffuse?: string;
  normal?: string;
}

interface TurretVisualMeshProps {
  visual: TurretVisual;
  previewUrls?: TurretPreviewTextureUrls;
  meshRef?: Ref<THREE.Mesh>;
  onBeforeCompile?: (shader: THREE.WebGLProgramParametersWithUniforms) => void;
  onPointerDown?: (e: THREE.Event) => void;
  /** Above hull (default 5). */
  renderOrder?: number;
  layerZ?: number;
}

export function TurretVisualMesh({
  visual,
  previewUrls,
  meshRef,
  onBeforeCompile,
  onPointerDown,
  renderOrder = 5,
  layerZ = 0,
}: TurretVisualMeshProps) {
  const [textures, setTextures] = useState<{
    diffuse?: THREE.Texture;
    normal?: THREE.Texture;
    loaded: boolean;
    error: boolean;
  }>({ loaded: false, error: false });

  const diffuseSrc = previewUrls?.diffuse ?? visual.texture;
  const normalSrc = previewUrls?.normal ?? visual.normalMap;

  useEffect(() => {
    let cancelled = false;
    const loader = new THREE.TextureLoader();

    const loadTex = (src: string | undefined) => {
      if (!src) return Promise.resolve<THREE.Texture | undefined>(undefined);
      if (src.startsWith('blob:')) {
        return new Promise<THREE.Texture | undefined>((resolve, reject) => {
          loader.load(src, resolve, undefined, reject);
        });
      }
      const url = assetUrl(src);
      return new Promise<THREE.Texture | undefined>((resolve, reject) => {
        loader.load(url, resolve, undefined, reject);
      });
    };

    setTextures({ loaded: false, error: false });

    Promise.all([loadTex(diffuseSrc), loadTex(normalSrc)])
      .then(([diffuse, normal]) => {
        if (cancelled) return;
        if (diffuse) {
          diffuse.colorSpace = THREE.SRGBColorSpace;
        }
        if (normal) {
          normal.colorSpace = THREE.LinearSRGBColorSpace;
        }
        setTextures({ diffuse, normal, loaded: true, error: !diffuse });
      })
      .catch(() => {
        if (!cancelled) setTextures({ loaded: true, error: true });
      });

    return () => {
      cancelled = true;
    };
  }, [diffuseSrc, normalSrc]);

  const [w, h] = visual.size;
  const roughness = visual.roughness ?? 0.4;
  const metalness = visual.metalness ?? 0.6;

  if (!textures.loaded) {
    return (
      <mesh position={[0, 0, layerZ]} renderOrder={renderOrder}>
        <planeGeometry args={[w, h]} />
        <meshBasicMaterial color="#334155" wireframe />
      </mesh>
    );
  }

  if (textures.error || !textures.diffuse) {
    return (
      <mesh position={[0, 0, layerZ]} renderOrder={renderOrder}>
        <planeGeometry args={[w, h]} />
        <meshStandardMaterial color="#475569" roughness={roughness} metalness={metalness} />
      </mesh>
    );
  }

  return (
    <mesh
      ref={meshRef}
      position={[0, 0, layerZ]}
      renderOrder={renderOrder}
      onPointerDown={onPointerDown}
    >
      <planeGeometry args={[w, h]} />
      <meshStandardMaterial
        onBeforeCompile={onBeforeCompile}
        map={textures.diffuse}
        normalMap={textures.normal}
        transparent
        alphaTest={0.1}
        depthWrite={false}
        roughness={roughness}
        metalness={metalness}
      />
    </mesh>
  );
}
