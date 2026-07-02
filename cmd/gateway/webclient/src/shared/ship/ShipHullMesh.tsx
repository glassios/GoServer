import { useEffect, useState, type Ref } from 'react';
import * as THREE from 'three';
import type { ShipHullDefinition } from '@/src/shared/types/ship';
import { assetUrl } from '@/src/shared/assetUrl';

export interface HullPreviewTextureUrls {
  diffuse?: string;
  normal?: string;
}

interface ShipHullMeshProps {
  hull: ShipHullDefinition;
  previewUrls?: HullPreviewTextureUrls;
  meshRef?: Ref<THREE.Mesh>;
  onBeforeCompile?: (shader: THREE.WebGLProgramParametersWithUniforms) => void;
  onPointerDown?: (e: THREE.Event) => void;
  /** Draw below weapon layers (orthographic: higher z = closer to camera). */
  layerZ?: number;
  renderOrder?: number;
}

export function ShipHullMesh({
  hull,
  previewUrls,
  meshRef,
  onBeforeCompile,
  onPointerDown,
  layerZ = 0,
  renderOrder = 0,
}: ShipHullMeshProps) {
  const [textures, setTextures] = useState<{
    diffuse?: THREE.Texture;
    normal?: THREE.Texture;
    loaded: boolean;
    error: boolean;
  }>({ loaded: false, error: false });

  const diffuseSrc = previewUrls?.diffuse ?? hull.texture;
  const normalSrc = previewUrls?.normal ?? hull.normalMap;

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
        setTextures({ diffuse, normal, loaded: true, error: !diffuse });
      })
      .catch(() => {
        if (!cancelled) setTextures({ loaded: true, error: true });
      });

    return () => {
      cancelled = true;
    };
  }, [diffuseSrc, normalSrc]);

  const [w, h] = hull.size;
  const roughness = hull.roughness ?? 0.4;
  const metalness = hull.metalness ?? 0.6;

  if (!textures.loaded) {
    return (
      <mesh rotation={[0, 0, Math.PI]}>
        <planeGeometry args={[w, h]} />
        <meshBasicMaterial color="#334155" wireframe />
      </mesh>
    );
  }

  if (textures.error || !textures.diffuse) {
    return (
      <mesh rotation={[0, 0, Math.PI]}>
        <planeGeometry args={[w, h]} />
        <meshStandardMaterial color="#475569" roughness={roughness} metalness={metalness} />
      </mesh>
    );
  }

  return (
    <mesh
      ref={meshRef}
      rotation={[0, 0, Math.PI]}
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
