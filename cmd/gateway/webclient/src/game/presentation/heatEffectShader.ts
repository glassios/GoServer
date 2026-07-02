import * as THREE from 'three';

export const HEAT_MAX_HITS = 50;
const HEAT_FADE_MS = 30000;

/** Hit position and radius in ship-local XY (same plane as hull + turrets). */
export interface HeatHit {
  x: number;
  y: number;
  time: number;
  radius: number;
}

export interface HeatUniforms {
  hitData: { value: Float32Array };
  hitCount: { value: number };
}

export interface ShipHeatShaderExtras {
  shipFromWorld: { value: THREE.Matrix4 };
}

export function createHeatUniforms(): HeatUniforms {
  return {
    hitData: { value: new Float32Array(200) },
    hitCount: { value: 0 },
  };
}

export function createShipFromWorldUniform(): ShipHeatShaderExtras['shipFromWorld'] {
  return { value: new THREE.Matrix4() };
}

export function updateHeatUniforms(uniforms: HeatUniforms, hits: HeatHit[]): void {
  const nowMs = performance.now();
  let currentHitCount = 0;
  const active: HeatHit[] = [];

  for (const hit of hits) {
    const age = (nowMs - hit.time) / HEAT_FADE_MS;
    if (age < 1.0) {
      const intensity = 1.0 - Math.pow(age, 0.5);
      uniforms.hitData.value[currentHitCount * 4] = hit.x;
      uniforms.hitData.value[currentHitCount * 4 + 1] = hit.y;
      uniforms.hitData.value[currentHitCount * 4 + 2] = Math.max(0, intensity);
      uniforms.hitData.value[currentHitCount * 4 + 3] = hit.radius;
      currentHitCount++;
      active.push(hit);
    }
  }

  hits.length = 0;
  hits.push(...active);
  uniforms.hitCount.value = currentHitCount;
}

export function appendHeatHit(hits: HeatHit[], hit: Omit<HeatHit, 'time'> & { time?: number }): void {
  hits.push({
    x: hit.x,
    y: hit.y,
    time: hit.time ?? performance.now(),
    radius: hit.radius,
  });
  if (hits.length > HEAT_MAX_HITS) hits.shift();
}

const HEAT_RADIUS_UV_MIN = 0.02;
const HEAT_RADIUS_UV_MAX = 0.04;

export function randomHeatShipRadius(hullSize: [number, number]): number {
  const t = Math.random();
  const uvRadius = HEAT_RADIUS_UV_MIN + t * (HEAT_RADIUS_UV_MAX - HEAT_RADIUS_UV_MIN);
  return uvRadius * Math.max(hullSize[0], hullSize[1]);
}

export function patchHeatShader(
  shader: THREE.WebGLProgramParametersWithUniforms,
  uniforms: HeatUniforms,
  extras: ShipHeatShaderExtras
): void {
  shader.uniforms.hitData = uniforms.hitData;
  shader.uniforms.hitCount = uniforms.hitCount;
  shader.uniforms.uShipFromWorld = extras.shipFromWorld;

  shader.vertexShader =
    `
        uniform mat4 uShipFromWorld;
        varying vec2 vShipLocalXY;
        varying vec2 vUvPosition;
      ` + shader.vertexShader;

  shader.vertexShader = shader.vertexShader.replace(
    `#include <uv_vertex>`,
    `#include <uv_vertex>
         vUvPosition = uv;
        `
  );

  // worldpos_vertex is empty unless shadows/envmap — use modelMatrix + transformed instead.
  shader.vertexShader = shader.vertexShader.replace(
    `#include <project_vertex>`,
    `#include <project_vertex>
         vec4 heatShipLocal = uShipFromWorld * modelMatrix * vec4( transformed, 1.0 );
         vShipLocalXY = heatShipLocal.xy;
        `
  );

  shader.fragmentShader =
    `
        uniform float hitData[200];
        uniform int hitCount;
        varying vec2 vShipLocalXY;
        varying vec2 vUvPosition;
      ` + shader.fragmentShader;

  shader.fragmentShader = shader.fragmentShader.replace(
    `#include <normal_fragment_maps>`,
    `#include <normal_fragment_maps>
            
            vec3 accumHeatColor = vec3(0.0);
            vec2 totalDent = vec2(0.0);
            
            for(int i = 0; i < 50; i++) {
                if (i >= hitCount) break;
                
                float hx = hitData[i * 4];
                float hy = hitData[i * 4 + 1];
                float intensity = hitData[i * 4 + 2];
                float radius = hitData[i * 4 + 3];
                
                vec2 center = vec2(hx, hy);
                vec2 pos = vShipLocalXY;
                
                vec2 diff = pos - center;
                float dist = length(diff);
                
                float heatSq = clamp(1.0 - (dist / radius), 0.0, 1.0);
                float heat = heatSq * heatSq * intensity;
                
                if (dist < radius && dist > 0.0001) {
                    float factor = 2.0 * heatSq * (1.0 / radius) / dist;
                    totalDent += diff * factor * intensity * 0.02; 
                }
                
                vec3 darkRed = vec3(0.3, 0.0, 0.0);
                vec3 red = vec3(1.0, 0.0, 0.0);
                vec3 brightRed = vec3(1.0, 0.2, 0.0);
                
                vec3 color = mix(darkRed, red, clamp(heat * 2.0, 0.0, 1.0));
                color = mix(color, brightRed, clamp((heat - 0.5) * 2.0, 0.0, 1.0));
                
                accumHeatColor += color * heat * 10.0; 
            }
            
            vec3 vSigmaX = dFdx( -vViewPosition );
            vec3 vSigmaY = dFdy( -vViewPosition );
            vec2 dUvX = dFdx( vUvPosition );
            vec2 dUvY = dFdy( vUvPosition );
            float fDet = dUvX.x * dUvY.y - dUvX.y * dUvY.x;
            if ( fDet != 0.0 ) {
                vec3 T = (  dUvY.y * vSigmaX - dUvX.y * vSigmaY ) / fDet;
                vec3 B = ( -dUvY.x * vSigmaX + dUvX.x * vSigmaY ) / fDet;
                normal = normalize( normal - T * totalDent.x - B * totalDent.y );
            }
            `
  );

  shader.fragmentShader = shader.fragmentShader.replace(
    `#include <emissivemap_fragment>`,
    `#include <emissivemap_fragment>
            totalEmissiveRadiance += accumHeatColor;
            `
  );
}
