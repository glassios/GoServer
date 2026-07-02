import { useFrame } from '@react-three/fiber';
import { useEffect, useMemo, useRef } from 'react';
import * as THREE from 'three';
import type { BlasterConfig, BlasterMuzzleSlot } from '@/src/shared/types/turret';
import { applyBlasterShaderUniforms } from './weaponShaderUniforms';
import { isFiring, type FiringInput } from './resolveFiring';

interface Bolt {
  id: number;
  x: number;
  y: number;
  z: number;
  spawnTime: number;
}

const _tempObject = new THREE.Object3D();

interface BlasterMuzzlePreviewProps {
  slot: BlasterMuzzleSlot;
  firing: FiringInput;
}

function MuzzleFlashLight({
  slot,
  flashRef,
}: {
  slot: BlasterMuzzleSlot;
  flashRef: React.MutableRefObject<number>;
}) {
  const lightRef = useRef<THREE.PointLight>(null);
  const pos = slot.lightPosition ?? slot.localPosition;
  const { pointLights } = slot;

  useFrame(() => {
    if (!lightRef.current) return;
    lightRef.current.intensity = THREE.MathUtils.lerp(
      lightRef.current.intensity,
      0,
      pointLights.decayLerp
    );
    if (flashRef.current > 0) {
      lightRef.current.intensity = pointLights.flashIntensity;
      flashRef.current = 0;
    }
  });

  return (
    <pointLight
      ref={lightRef}
      position={[pos.x, pos.y, pos.z]}
      color={pointLights.color}
      intensity={0}
      distance={pointLights.distance}
      decay={pointLights.decay}
    />
  );
}

function BlasterMuzzlePreview({ slot, firing }: BlasterMuzzlePreviewProps) {
  const meshRef = useRef<THREE.InstancedMesh>(null);
  const boltsRef = useRef<Bolt[]>([]);
  const flashRef = useRef(0);
  const lastFireTime = useRef(0);
  const { colors, shader, boltSize, maxBolts, boltSpeed, boltLifetime, fireInterval } = slot;

  const uniforms = useMemo(
    () => ({
      uCoreColor: { value: new THREE.Vector3(colors.core.r, colors.core.g, colors.core.b) },
      uGlowColor: { value: new THREE.Vector3(colors.glow.r, colors.glow.g, colors.glow.b) },
      uAlphaPowX: { value: shader.alphaPowX },
      uAlphaPowY: { value: shader.alphaPowY },
      uGlowMix: { value: shader.glowMix },
      uColorMultiply: { value: shader.colorMultiply },
    }),
    [colors, shader]
  );

  const syncMaterialUniforms = () => {
    const mat = meshRef.current?.material;
    if (mat && 'uniforms' in mat) {
      applyBlasterShaderUniforms(mat as THREE.ShaderMaterial, colors, shader);
    }
  };

  useEffect(() => {
    syncMaterialUniforms();
  }, [colors, shader]);

  const vertexShader = `
    varying vec2 vUv;
    void main() {
      vUv = uv;
      gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0);
    }
  `;

  const fragmentShader = `
    varying vec2 vUv;
    uniform vec3 uCoreColor;
    uniform vec3 uGlowColor;
    uniform float uAlphaPowX;
    uniform float uAlphaPowY;
    uniform float uGlowMix;
    uniform float uColorMultiply;
    void main() {
      float alphaX = 1.0 - pow(abs(vUv.x - 0.5) * 2.0, uAlphaPowX);
      float distY = abs(vUv.y - 0.5) * 2.0;
      float alphaY = 1.0 - pow(distY, uAlphaPowY);
      float alpha = alphaX * alphaY;
      vec3 color = mix(uCoreColor, uGlowColor, distY * uGlowMix) * uColorMultiply;
      gl_FragColor = vec4(color, alpha);
    }
  `;

  useFrame((state, delta) => {
    syncMaterialUniforms();
    const now = state.clock.elapsedTime;

    if (isFiring(firing) && now - lastFireTime.current > fireInterval) {
      lastFireTime.current = now;
      flashRef.current = 1;
      boltsRef.current.push({
        id: Math.random(),
        x: slot.localPosition.x,
        y: slot.localPosition.y,
        z: slot.localPosition.z,
        spawnTime: now,
      });
    }

    if (!meshRef.current) return;

    boltsRef.current = boltsRef.current.filter((b) => now - b.spawnTime < boltLifetime);

    boltsRef.current.forEach((bolt) => {
      bolt.x += boltSpeed * delta;
    });

    meshRef.current.count = boltsRef.current.length;

    boltsRef.current.forEach((bolt, i) => {
      _tempObject.position.set(bolt.x, bolt.y, bolt.z);
      _tempObject.rotation.set(0, 0, 0);
      _tempObject.updateMatrix();
      meshRef.current!.setMatrixAt(i, _tempObject.matrix);
    });

    meshRef.current.instanceMatrix.needsUpdate = true;
  });

  return (
    <group>
      <instancedMesh
        ref={meshRef}
        args={[undefined, undefined, maxBolts]}
        frustumCulled={false}
        renderOrder={15}
      >
        <planeGeometry args={boltSize} />
        <shaderMaterial
          uniforms={uniforms}
          vertexShader={vertexShader}
          fragmentShader={fragmentShader}
          transparent
          depthWrite={false}
          blending={THREE.AdditiveBlending}
        />
      </instancedMesh>
      <MuzzleFlashLight slot={slot} flashRef={flashRef} />
    </group>
  );
}

interface BlasterWeaponPreviewProps {
  config: BlasterConfig;
  firing: FiringInput;
}

export function BlasterWeaponPreview({ config, firing }: BlasterWeaponPreviewProps) {
  return (
    <group>
      {config.muzzles.map((slot) => (
        <BlasterMuzzlePreview key={slot.id} slot={slot} firing={firing} />
      ))}
    </group>
  );
}
