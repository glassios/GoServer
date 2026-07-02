import type { EngineModeId } from '@/src/shared/types/engine';
import type { ShipEngineSlot } from '@/src/shared/types/ship';

export type InputMap = Record<string, boolean>;

export function resolveEngineModeFromBindings(
  slot: ShipEngineSlot,
  inputs: InputMap,
  fallback: EngineModeId = 'idle'
): EngineModeId {
  const bindings = slot.inputBindings;
  if (!bindings) return fallback;

  const anyPressed = (keys?: string[]) =>
    keys?.some((k) => inputs[k]) ?? false;

  if (anyPressed(bindings.reverse)) {
    return 'reverse';
  }

  if (anyPressed(bindings.thrust) || anyPressed(bindings.turnAssistLeft) || anyPressed(bindings.turnAssistRight)) {
    return 'thrust';
  }

  return fallback;
}
