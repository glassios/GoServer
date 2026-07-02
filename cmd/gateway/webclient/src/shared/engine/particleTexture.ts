import * as THREE from 'three';
import type { EngineDefinition } from '@/src/shared/types/engine';

export function createParticleTextureFromConfig(
  flame: EngineDefinition['flame']
): THREE.CanvasTexture {
  const size = flame.canvasSize ?? 64;
  const canvas = document.createElement('canvas');
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext('2d');
  if (ctx) {
    const half = size / 2;
    const gradient = ctx.createRadialGradient(half, half, 0, half, half, half);
    const sorted = [...flame.stops].sort((a, b) => a.position - b.position);
    for (const stop of sorted) {
      const [r, g, b, a] = stop.rgba;
      gradient.addColorStop(stop.position, `rgba(${r}, ${g}, ${b}, ${a})`);
    }
    ctx.fillStyle = gradient;
    ctx.fillRect(0, 0, size, size);
  }
  const texture = new THREE.CanvasTexture(canvas);
  texture.needsUpdate = true;
  return texture;
}
