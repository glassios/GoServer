// GoServer 3D client (React Three Fiber), served by the gateway at /client3d/.
// One client, two modes auto-selected by the player's current system_id:
//   - open world  → schematic SpaceScene (fleets/NPCs/objects as icons)
//   - battle room → high-fidelity BattleScene (Starsector-style instanced combat)
import { Canvas } from '@react-three/fiber';
import { Suspense, useEffect, useState } from 'react';
import { EndlessBackground, Lights } from './scene/SceneShell';
import { ModeRouter } from './scene/ModeRouter';
import { connection } from './net/session';
import { useNetKeyboard } from './net/useNetKeyboard';
import { useSystemId } from './net/hooks';
import { isBattleSystem } from './net/coords';
import type { ConnectionStatus } from './net/connection';

function StatusOverlay({ status, isBattle }: { status: ConnectionStatus; isBattle: boolean }) {
  const label: Record<ConnectionStatus, string> = {
    connecting: 'connecting…',
    open: 'authenticating…',
    authed: `online · ${connection.pilotName}`,
    closed: 'disconnected',
  };
  const color = status === 'authed' ? 'text-green-400' : status === 'closed' ? 'text-red-400' : 'text-cyan-300';
  return (
    <div className="absolute top-4 left-4 z-10 font-mono pointer-events-none text-sm">
      <span className="font-bold text-cyan-400">GoServer · 3D client</span>
      <span className={`ml-2 px-1.5 py-0.5 rounded text-xs ${isBattle ? 'bg-rose-600/70 text-white' : 'bg-cyan-700/50 text-cyan-100'}`}>
        {isBattle ? 'БОЙ' : 'КОСМОС'}
      </span>
      <span className="opacity-60"> · </span>
      <span className={color}>{label[status]}</span>
      <div className="opacity-50 text-xs mt-1">
        {isBattle ? 'наблюдение за боем · scroll — зум' : 'WASD — полёт · клик по маркеру боя — вступить · scroll — зум'}
      </div>
    </div>
  );
}

export default function App() {
  const [status, setStatus] = useState<ConnectionStatus>('closed');
  const systemId = useSystemId();
  const isBattle = isBattleSystem(systemId);

  useNetKeyboard(!isBattle);

  useEffect(() => {
    connection.onStatus = setStatus;
    connection.connect();
    return () => {
      connection.onStatus = undefined;
    };
  }, []);

  return (
    <div className="w-full h-screen bg-slate-950 overflow-hidden select-none relative touch-none">
      <StatusOverlay status={status} isBattle={isBattle} />
      <Canvas orthographic camera={{ zoom: 40, position: [0, 0, 100] }}>
        <color attach="background" args={['#020617']} />
        <Suspense fallback={null}>
          <EndlessBackground />
          <Lights />
          <ModeRouter />
        </Suspense>
      </Canvas>
    </div>
  );
}
