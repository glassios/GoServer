import { useFBO } from '@react-three/drei';
import { useFrame, useThree, type ThreeEvent } from '@react-three/fiber';
import { useMemo, useRef, type RefObject } from 'react';
import * as THREE from 'three';
import { gameUiState } from '@/src/game/input/gameInput';
import { appendRippleHit, RIPPLE_MAX_HITS } from '@/src/game/presentation/shipRipple';

interface PlayerShieldProps {
  shipGroupRef: RefObject<THREE.Group | null>;
}

const SHIELD_PLANE_SIZE = 14;
const SHIELD_RING_RADIUS = 4.2;
const SHIELD_GLOW_THICKNESS = 3.4;
const SHIELD_CORE_THICKNESS = 0.15;

export function PlayerShield({ shipGroupRef }: PlayerShieldProps) {
  const shieldRef = useRef<THREE.Mesh>(null);
  const materialRef = useRef<THREE.ShaderMaterial>(null);
  const power = useRef(0);
  const hitData = useMemo(() => new Float32Array(RIPPLE_MAX_HITS * 4), []);
  const shipFromWorld = useMemo(() => new THREE.Matrix4(), []);

  const { gl, scene, camera, size } = useThree();
  const fbo = useFBO(size.width * 0.5, size.height * 0.5);

  useFrame((state, delta) => {
    if (!shieldRef.current || !materialRef.current) return;

    if (gameUiState.shieldActive) {
      power.current = Math.min(power.current + delta / 3, 1);
    } else {
      power.current = Math.max(power.current - delta / 3, 0);
    }

    const hasRipples = gameUiState.rippleHits.length > 0;
    shieldRef.current.visible = power.current > 0 || hasRipples;

    const shipGroup = shipGroupRef.current;
    if (shipGroup) {
      shipFromWorld.copy(shipGroup.matrixWorld).invert();
      materialRef.current.uniforms.uShipFromWorld.value.copy(shipFromWorld);
    }

    const now = performance.now();
    let hitCount = 0;
    gameUiState.rippleHits.forEach((hit) => {
      const age = (now - hit.time) / 1000.0;
      if (age < 1.0) {
        hitData[hitCount * 4] = hit.x;
        hitData[hitCount * 4 + 1] = hit.y;
        hitData[hitCount * 4 + 2] = age;
        hitData[hitCount * 4 + 3] = hit.scale;
        hitCount++;
      }
    });
    gameUiState.rippleHits = gameUiState.rippleHits.filter((hit) => (now - hit.time) / 1000.0 < 1.0);

    if (shieldRef.current.visible) {
      shieldRef.current.visible = false;
      gl.setRenderTarget(fbo);
      gl.render(scene, camera);
      gl.setRenderTarget(null);
      shieldRef.current.visible = true;
    }

    const uniforms = materialRef.current.uniforms;
    uniforms.uPower.value = power.current;
    uniforms.uTime.value = state.clock.elapsedTime;
    uniforms.uHits.value = hitData;
    uniforms.uHitCount.value = hitCount;
    uniforms.uSceneTex.value = fbo.texture;
    uniforms.uResolution.value.set(size.width, size.height);
  });

  const uniforms = useMemo(
    () => ({
      uPower: { value: 0 },
      uTime: { value: 0 },
      uColor1: { value: new THREE.Color('#ff5500') },
      uColor2: { value: new THREE.Color('#ff9900') },
      uHits: { value: hitData },
      uHitCount: { value: 0 },
      uSceneTex: { value: null as THREE.Texture | null },
      uResolution: { value: new THREE.Vector2() },
      uShipFromWorld: { value: new THREE.Matrix4() },
    }),
    [hitData]
  );

  const vertexShader = /* glsl */ `
    uniform mat4 uShipFromWorld;
    varying vec2 vUv;
    varying vec4 vScreenPos;
    varying vec2 vShipLocalXY;

    void main() {
      vUv = uv;
      vec4 worldPos = modelMatrix * vec4(position, 1.0);
      vec4 mvPosition = viewMatrix * worldPos;
      gl_Position = projectionMatrix * mvPosition;
      vScreenPos = gl_Position;
      vShipLocalXY = (uShipFromWorld * worldPos).xy;
    }
  `;

  const fragmentShader = /* glsl */ `
    uniform float uPower;
    uniform float uTime;
    uniform vec3 uColor1;
    uniform vec3 uColor2;
    uniform float uHits[40];
    uniform int uHitCount;
    uniform sampler2D uSceneTex;
    uniform vec2 uResolution;
    varying vec2 vUv;
    varying vec4 vScreenPos;
    varying vec2 vShipLocalXY;

    void main() {
      vec2 screenUV = (vScreenPos.xy / vScreenPos.w) * 0.5 + 0.5;
      vec2 finalDistortion = vec2(0.0);
      vec2 localPos = vShipLocalXY;

      for (int i = 0; i < ${RIPPLE_MAX_HITS}; i++) {
        if (i >= uHitCount) break;
        float hx = uHits[i * 4];
        float hy = uHits[i * 4 + 1];
        float age = uHits[i * 4 + 2];
        float hitScale = uHits[i * 4 + 3];
        vec2 center = vec2(hx, hy);
        vec2 diff = localPos - center;
        float hd = length(diff);
        float sr = age * 4.0 * hitScale;
        float sThick = 0.4 * hitScale;
        float sDist = abs(hd - sr);
        if (sDist < sThick && hd > 0.01) {
          float sInt = 1.0 - (sDist / sThick);
          float fade = pow(1.0 - age, 2.0);
          sInt *= fade;
          vec2 dir = normalize(diff);
          float lens = (hd - sr) / sThick;
          finalDistortion += dir * lens * fade * 0.05 * hitScale;
        }
      }

      vec2 distortedUV = screenUV + finalDistortion;
      vec4 bgColor = texture2D(uSceneTex, distortedUV);

      vec2 shieldPos = vShipLocalXY - finalDistortion * 2.0;
      float dist = length(shieldPos);
      float radius = ${SHIELD_RING_RADIUS.toFixed(1)};
      float distFromRing = abs(dist - radius);
      float glowThickness = ${SHIELD_GLOW_THICKNESS.toFixed(1)};
      float coreThickness = ${SHIELD_CORE_THICKNESS.toFixed(2)};

      float intensity = 0.0;
      vec3 color = vec3(0.0);
      if (distFromRing < glowThickness) {
        float glow = 1.0 - (distFromRing / glowThickness);
        glow = pow(glow, 2.0) * 0.4;
        float core = 0.0;
        if (distFromRing < coreThickness) {
          core = pow(1.0 - (distFromRing / coreThickness), 2.0);
        }
        intensity = glow + core;
        color = mix(uColor1, uColor2, smoothstep(0.0, 1.0, core * 2.0));
      }

      float angle = atan(shieldPos.y, shieldPos.x);
      float maxAngle = uPower * 3.14159265;
      float absAngle = abs(angle);
      float arcAlpha = 1.0;
      if (absAngle > maxAngle) {
        arcAlpha = 0.0;
      } else {
        float edgeDist = maxAngle - absAngle;
        arcAlpha = smoothstep(0.0, 0.2, edgeDist);
      }

      float pulse = 0.9 + 0.1 * sin(uTime * 5.0 - absAngle * 2.0);
      intensity *= pulse;
      float finalAlpha = intensity * arcAlpha;
      vec3 finalColor = color * intensity * arcAlpha * 2.5;

      if (length(finalDistortion) > 0.001) {
        gl_FragColor = vec4(bgColor.rgb + finalColor, max(bgColor.a, finalAlpha));
      } else {
        gl_FragColor = vec4(finalColor, finalAlpha);
      }
    }
  `;

  const handlePointerDown = (e: ThreeEvent<PointerEvent>) => {
    if (!gameUiState.shieldActive) return;
    e.stopPropagation();

    const shipGroup = shipGroupRef.current;
    if (!shipGroup) return;

    const shipLocal = shipGroup.worldToLocal(e.point.clone());
    let x = shipLocal.x;
    let y = shipLocal.y;
    const dist = Math.hypot(x, y);
    const isEdge = dist > 3.0;
    const hitScale = isEdge ? 0.4 : 1.0;

    if (isEdge && dist > 0.1) {
      x = (x / dist) * SHIELD_RING_RADIUS;
      y = (y / dist) * SHIELD_RING_RADIUS;
    }

    appendRippleHit(gameUiState.rippleHits, { x, y, scale: hitScale });
  };

  return (
    <mesh
      ref={shieldRef}
      position={[0, 0, 0.1]}
      renderOrder={30}
      onPointerDown={handlePointerDown}
    >
      <planeGeometry args={[SHIELD_PLANE_SIZE, SHIELD_PLANE_SIZE]} />
      <shaderMaterial
        ref={materialRef}
        uniforms={uniforms}
        vertexShader={vertexShader}
        fragmentShader={fragmentShader}
        transparent
        depthWrite={false}
        depthTest={false}
        blending={THREE.NormalBlending}
        side={THREE.DoubleSide}
      />
    </mesh>
  );
}
