import type { BeamEmitterSlot, BlasterMuzzleSlot, RgbColor } from '@/src/shared/types/turret';
import * as THREE from 'three';

function rgbToVec3(c: RgbColor): THREE.Vector3 {
  return new THREE.Vector3(c.r, c.g, c.b);
}

type ShaderMaterialLike = THREE.ShaderMaterial | null | undefined;

export function applyBeamShaderUniforms(
  material: ShaderMaterialLike,
  colors: BeamEmitterSlot['colors'],
  shader: BeamEmitterSlot['shader']
): void {
  if (!material?.uniforms) return;
  const u = material.uniforms;
  if (u.uCoreColor) u.uCoreColor.value.copy(rgbToVec3(colors.core));
  if (u.uGlowColor) u.uGlowColor.value.copy(rgbToVec3(colors.glow));
  if (u.uAlphaPowX) u.uAlphaPowX.value = shader.alphaPowX;
  if (u.uAlphaPowY) u.uAlphaPowY.value = shader.alphaPowY;
  if (u.uGlowMix) u.uGlowMix.value = shader.glowMix;
}

export function applyBlasterShaderUniforms(
  material: ShaderMaterialLike,
  colors: BlasterMuzzleSlot['colors'],
  shader: BlasterMuzzleSlot['shader']
): void {
  if (!material?.uniforms) return;
  const u = material.uniforms;
  if (u.uCoreColor) u.uCoreColor.value.copy(rgbToVec3(colors.core));
  if (u.uGlowColor) u.uGlowColor.value.copy(rgbToVec3(colors.glow));
  if (u.uAlphaPowX) u.uAlphaPowX.value = shader.alphaPowX;
  if (u.uAlphaPowY) u.uAlphaPowY.value = shader.alphaPowY;
  if (u.uGlowMix) u.uGlowMix.value = shader.glowMix;
  if (u.uColorMultiply) u.uColorMultiply.value = shader.colorMultiply;
}
