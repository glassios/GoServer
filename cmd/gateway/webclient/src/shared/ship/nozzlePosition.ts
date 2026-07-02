import type { EngineDefinition } from '@/src/shared/types/engine';
import { mergeEngineMount } from '@/src/shared/engine/defaults';

export function getNozzleWorldPosition(config: EngineDefinition): [number, number, number] {
  const { lateralOffset, localPosition } = config.mount;
  const backOffset = config.particle.backOffset;
  return [-backOffset + localPosition.x, lateralOffset + localPosition.y, localPosition.z];
}

export function mountOverrideFromMarker(
  markerX: number,
  markerY: number,
  baseEngine: EngineDefinition,
  currentOverride?: Partial<EngineDefinition['mount']>
): Partial<EngineDefinition['mount']> {
  const merged = mergeEngineMount(baseEngine, currentOverride);
  return {
    ...currentOverride,
    side: merged.side,
    lateralOffset: markerY - baseEngine.mount.localPosition.y,
    localPosition: {
      ...baseEngine.mount.localPosition,
      ...currentOverride?.localPosition,
      x: markerX + baseEngine.particle.backOffset,
      y: merged.localPosition.y,
      z: merged.localPosition.z,
    },
  };
}
