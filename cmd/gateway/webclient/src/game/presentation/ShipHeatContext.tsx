import { useFrame } from '@react-three/fiber';
import type { ThreeEvent } from '@react-three/fiber';
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  type ReactNode,
  type RefObject,
} from 'react';
import * as THREE from 'three';
import { gameUiState } from '@/src/game/input/gameInput';
import {
  appendHeatHit,
  createHeatUniforms,
  createShipFromWorldUniform,
  patchHeatShader,
  randomHeatShipRadius,
  updateHeatUniforms,
  type HeatHit,
  type HeatUniforms,
  type ShipHeatShaderExtras,
} from '@/src/game/presentation/heatEffectShader';

interface ShipHeatContextValue {
  heatUniforms: HeatUniforms;
  shipFromWorld: ShipHeatShaderExtras['shipFromWorld'];
  createOnBeforeCompile: () => (shader: THREE.WebGLProgramParametersWithUniforms) => void;
  applyHeatFromPointer: (e: ThreeEvent<PointerEvent>) => void;
}

const ShipHeatContext = createContext<ShipHeatContextValue | null>(null);

interface ShipHeatProviderProps {
  children: ReactNode;
  shipGroupRef: RefObject<THREE.Group | null>;
  hullSize: [number, number];
}

export function ShipHeatProvider({ children, shipGroupRef, hullSize }: ShipHeatProviderProps) {
  const heatUniforms = useMemo(() => createHeatUniforms(), []);
  const shipFromWorld = useMemo(() => createShipFromWorldUniform(), []);
  const hitsRef = useRef<HeatHit[]>([]);
  const hullSizeRef = useRef(hullSize);
  hullSizeRef.current = hullSize;

  const shipFromWorldMatrix = shipFromWorld.value;

  const createOnBeforeCompile = useCallback(() => {
    const extras: ShipHeatShaderExtras = { shipFromWorld };
    return (shader: THREE.WebGLProgramParametersWithUniforms) => {
      patchHeatShader(shader, heatUniforms, extras);
    };
  }, [heatUniforms, shipFromWorld]);

  const applyHeatFromPointer = useCallback(
    (e: ThreeEvent<PointerEvent>) => {
      if (gameUiState.shieldActive) return;

      const shipGroup = shipGroupRef.current;
      if (!shipGroup) return;

      const shipLocal = shipGroup.worldToLocal(e.point.clone());
      const radius = randomHeatShipRadius(hullSizeRef.current);

      appendHeatHit(hitsRef.current, {
        x: shipLocal.x,
        y: shipLocal.y,
        radius,
      });
    },
    [shipGroupRef]
  );

  useFrame(() => {
    const shipGroup = shipGroupRef.current;
    if (shipGroup) {
      shipFromWorldMatrix.copy(shipGroup.matrixWorld).invert();
    }
    updateHeatUniforms(heatUniforms, hitsRef.current);
  });

  const value = useMemo(
    (): ShipHeatContextValue => ({
      heatUniforms,
      shipFromWorld,
      createOnBeforeCompile,
      applyHeatFromPointer,
    }),
    [heatUniforms, shipFromWorld, createOnBeforeCompile, applyHeatFromPointer]
  );

  return <ShipHeatContext.Provider value={value}>{children}</ShipHeatContext.Provider>;
}

export function useShipHeat(): ShipHeatContextValue {
  const ctx = useContext(ShipHeatContext);
  if (!ctx) throw new Error('useShipHeat must be used within ShipHeatProvider');
  return ctx;
}

export function useCombatHeatMaterial() {
  const ctx = useShipHeat();
  const onBeforeCompile = useMemo(() => ctx.createOnBeforeCompile(), [ctx]);

  const handlePointerDown = useCallback(
    (e: ThreeEvent<PointerEvent>) => {
      e.stopPropagation();
      ctx.applyHeatFromPointer(e);
    },
    [ctx]
  );

  return { onBeforeCompile, handlePointerDown };
}
