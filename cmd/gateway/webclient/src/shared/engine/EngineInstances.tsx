import { useEffect, useMemo, useState } from 'react';
import type { EngineDefinition, EngineModeId } from '@/src/shared/types/engine';
import type { ShipEngineSlot } from '@/src/shared/types/ship';
import { mergeEngineMount } from './defaults';
import { loadEngineDefinition } from './loadEngine';
import { resolveEngineModeFromBindings, type InputMap } from './resolveMode';
import { EngineParticles } from './EngineParticles';
import type { WorldTransform } from './simulator';

export interface ResolvedEngineInstance {
  slot: ShipEngineSlot;
  config: EngineDefinition;
}

interface EngineInstancesProps {
  slots: ShipEngineSlot[];
  transform: WorldTransform;
  inputs: InputMap;
  /** When set, overrides binding-based mode for all instances (editor preview) */
  previewMode?: EngineModeId;
}

export function EngineInstances({
  slots,
  transform,
  inputs,
  previewMode,
}: EngineInstancesProps) {
  const [instances, setInstances] = useState<ResolvedEngineInstance[]>([]);

  const slotKey = useMemo(() => slots.map((s) => s.id).join(','), [slots]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const loaded: ResolvedEngineInstance[] = [];
      for (const slot of slots) {
        const base = await loadEngineDefinition(slot.engineAsset);
        if (cancelled) return;
        const mount = mergeEngineMount(base, slot.mountOverride);
        loaded.push({
          slot,
          config: { ...base, mount },
        });
      }
      if (!cancelled) setInstances(loaded);
    })();
    return () => {
      cancelled = true;
    };
  }, [slotKey, slots]);

  if (instances.length === 0) return null;

  return (
    <>
      {instances.map(({ slot, config }) => {
        const resolveMode = (): EngineModeId =>
          previewMode ?? resolveEngineModeFromBindings(slot, inputs, 'idle');
        const lightY =
          slot.mountOverride?.side === 'right'
            ? -Math.abs(config.pointLight.position.y)
            : slot.mountOverride?.side === 'left'
              ? Math.abs(config.pointLight.position.y)
              : config.pointLight.position.y;
        const lightConfig: EngineDefinition = {
          ...config,
          pointLight: {
            ...config.pointLight,
            position: {
              ...config.pointLight.position,
              y: lightY,
            },
          },
        };
        return (
          <EngineParticles
            key={slot.id}
            config={lightConfig}
            activeMode={resolveMode}
            transform={transform}
          />
        );
      })}
    </>
  );
}
