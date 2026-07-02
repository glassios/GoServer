import { useFrame } from '@react-three/fiber';

import { useEffect, useMemo, useRef } from 'react';

import * as THREE from 'three';

import type { BeamConfig, BeamEmitterSlot } from '@/src/shared/types/turret';

import type { Vec3 } from '@/src/shared/types/engine';

import { applyBeamShaderUniforms } from './weaponShaderUniforms';
import { isFiring, type FiringInput } from './resolveFiring';



function lightPosForSlot(slot: BeamEmitterSlot): Vec3 {

  return (

    slot.lightPosition ?? {

      x: slot.localPosition.x + 0.3,

      y: slot.localPosition.y,

      z: slot.localPosition.z + 0.5,

    }

  );

}



interface BeamEmitterProps {
  slot: BeamEmitterSlot;
  firing: FiringInput;
}

function BeamEmitter({ slot, firing }: BeamEmitterProps) {

  const laserMeshRef = useRef<THREE.Mesh>(null);

  const laserMaterialRef = useRef<THREE.ShaderMaterial>(null);

  const lightRef = useRef<THREE.PointLight>(null);

  const intensityAcc = useRef(0);

  const { ramp, intensity, colors, shader, pointLight, beamLength, beamWidth } = slot;

  const meshX = slot.localPosition.x + beamLength / 2;

  const lightPos = lightPosForSlot(slot);



  const uniforms = useMemo(

    () => ({

      uIntensity: { value: 0 },

      uCoreColor: { value: new THREE.Vector3(colors.core.r, colors.core.g, colors.core.b) },

      uGlowColor: { value: new THREE.Vector3(colors.glow.r, colors.glow.g, colors.glow.b) },

      uAlphaPowX: { value: shader.alphaPowX },

      uAlphaPowY: { value: shader.alphaPowY },

      uGlowMix: { value: shader.glowMix },

    }),

    [colors, shader]

  );



  useEffect(() => {

    applyBeamShaderUniforms(laserMaterialRef.current, colors, shader);

  }, [colors, shader]);



  useFrame((_, delta) => {

    if (!laserMeshRef.current || !laserMaterialRef.current) return;

    applyBeamShaderUniforms(laserMaterialRef.current, colors, shader);



    const active = isFiring(firing);

    if (active) {
      intensityAcc.current = Math.min(intensityAcc.current + delta * ramp.rampUp, 1);
    } else {
      intensityAcc.current = Math.max(intensityAcc.current - delta * ramp.rampDown, 0);
    }



    let shaderIntensity = intensityAcc.current;

    if (laserMaterialRef.current.uniforms) {

      laserMaterialRef.current.uniforms.uIntensity.value = shaderIntensity;

    }

    laserMeshRef.current.visible = intensityAcc.current > 0;



    let lightIntensity = intensityAcc.current * intensity.lightMultiplier;

    if (active && intensityAcc.current === 1) {

      shaderIntensity =

        intensity.jitterShaderMin +

        Math.random() * (intensity.jitterShaderMax - intensity.jitterShaderMin);

      laserMaterialRef.current.uniforms.uIntensity.value = shaderIntensity;

      lightIntensity += Math.random() * intensity.jitterLightExtra;

    }



    if (lightRef.current && pointLight.enabled) {

      lightRef.current.intensity = lightIntensity;

    } else if (lightRef.current) {

      lightRef.current.intensity = 0;

    }

  });



  const vertexShader = `

    varying vec2 vUv;

    void main() {

      vUv = uv;

      gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);

    }

  `;



  const fragmentShader = `

    varying vec2 vUv;

    uniform float uIntensity;

    uniform vec3 uCoreColor;

    uniform vec3 uGlowColor;

    uniform float uAlphaPowX;

    uniform float uAlphaPowY;

    uniform float uGlowMix;

    void main() {

      float alphaX = 1.0 - pow(vUv.x, uAlphaPowX);

      float distY = abs(vUv.y - 0.5) * 2.0;

      float alphaY = 1.0 - pow(distY, uAlphaPowY);

      float alpha = alphaX * alphaY * uIntensity;

      vec3 color = mix(uCoreColor, uGlowColor, distY * uGlowMix);

      gl_FragColor = vec4(color, alpha);

    }

  `;



  return (

    <group>

      <mesh
        ref={laserMeshRef}
        position={[meshX, slot.localPosition.y, slot.localPosition.z]}
        visible={false}
        renderOrder={15}
      >

        <planeGeometry args={[beamLength, beamWidth]} />

        <shaderMaterial

          ref={laserMaterialRef}

          uniforms={uniforms}

          vertexShader={vertexShader}

          fragmentShader={fragmentShader}

          transparent

          depthWrite={false}

          blending={THREE.AdditiveBlending}

        />

      </mesh>

      {pointLight.enabled && (

        <pointLight

          ref={lightRef}

          position={[lightPos.x, lightPos.y, lightPos.z]}

          color={pointLight.color}

          intensity={0}

          distance={pointLight.distance}

          decay={pointLight.decay}

        />

      )}

    </group>

  );

}



interface BeamWeaponProps {
  config: BeamConfig;
  firing: FiringInput;
}



export function BeamWeapon({ config, firing }: BeamWeaponProps) {

  return (

    <group>

      {config.emitters.map((slot) => (

        <BeamEmitter key={slot.id} slot={slot} firing={firing} />

      ))}

    </group>

  );

}


