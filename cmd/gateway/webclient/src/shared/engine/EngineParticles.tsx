import { useFrame } from '@react-three/fiber';
import { useEffect, useMemo, useRef } from 'react';
import * as THREE from 'three';
import type { EngineDefinition, EngineModeId } from '@/src/shared/types/engine';
import type { WorldTransform } from './simulator';
import { EngineSimulator } from './simulator';

export type EngineModeSource = EngineModeId | (() => EngineModeId);

function resolveEngineMode(source: EngineModeSource): EngineModeId {
  return typeof source === 'function' ? source() : source;
}

interface EngineParticlesProps {
  config: EngineDefinition;
  activeMode: EngineModeSource;
  transform: WorldTransform;
  showLight?: boolean;
}

export function EngineParticles({
  config,
  activeMode,
  transform,
  showLight = true,
}: EngineParticlesProps) {
  const meshRef = useRef<THREE.InstancedMesh>(null);
  const materialRef = useRef<THREE.MeshBasicMaterial>(null);
  const lightRef = useRef<THREE.PointLight>(null);
  const simulatorRef = useRef<EngineSimulator | null>(null);

  const simulator = useMemo(() => {
    simulatorRef.current?.dispose();
    const sim = new EngineSimulator(config);
    simulatorRef.current = sim;
    return sim;
  }, []);

  useEffect(() => {
    simulator.setConfig(config);
    if (materialRef.current) {
      materialRef.current.map = simulator.getTexture();
      materialRef.current.needsUpdate = true;
    }
  }, [config, simulator]);

  useEffect(() => {
    return () => {
      simulatorRef.current?.dispose();
      simulatorRef.current = null;
    };
  }, [simulator]);

  const pl = config.pointLight;

  useFrame((_, delta) => {
    simulator.tick(delta, transform, resolveEngineMode(activeMode));
    if (meshRef.current) {
      simulator.writeInstanceMatrices(meshRef.current);
    }
    if (lightRef.current && pl.enabled && showLight) {
      lightRef.current.intensity = simulator.getLightIntensity();
    }
  });

  return (
    <group>
      <instancedMesh
        ref={meshRef}
        args={[undefined, undefined, config.particle.maxCount]}
        renderOrder={2}
        frustumCulled={false}
      >
        <planeGeometry args={[config.particle.quadSize, config.particle.quadSize]} />
        <meshBasicMaterial
          ref={materialRef}
          map={simulator.getTexture()}
          color="#ffffff"
          transparent
          opacity={config.particle.opacity}
          blending={THREE.AdditiveBlending}
          depthWrite={false}
        />
      </instancedMesh>
      {pl.enabled && showLight && (
        <pointLight
          ref={lightRef}
          position={[pl.position.x, pl.position.y, pl.position.z]}
          color={pl.color}
          intensity={0}
          distance={pl.distance}
          decay={pl.decay}
        />
      )}
    </group>
  );
}
