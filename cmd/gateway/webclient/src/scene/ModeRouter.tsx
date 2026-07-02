// Picks the renderer for the player's current system: schematic SpaceScene in the open world,
// high-fidelity BattleScene inside a combat instance (system_id >= BATTLE_SYSTEM_THRESHOLD).
import { isBattleSystem } from '@/src/net/coords';
import { useSystemId } from '@/src/net/hooks';
import { CameraController } from './SceneShell';
import { SpaceScene } from './space/SpaceScene';
import { BattleScene } from './battle/BattleScene';

export function ModeRouter() {
  const systemId = useSystemId();
  if (isBattleSystem(systemId)) {
    return <BattleScene />;
  }
  return (
    <>
      <CameraController />
      <SpaceScene />
    </>
  );
}
