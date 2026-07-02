import { useEffect } from 'react';
import { gameInput, gameUiState } from '@/src/game/input/gameInput';

export function useGameKeyboard(): void {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      let key = e.key.toLowerCase();
      if (key === ' ') key = 'fire';
      if (key === 'b') key = 'blaster';
      if (key === 'v') {
        gameUiState.shieldActive = !gameUiState.shieldActive;
      }
      if (key in gameInput) {
        gameInput[key as keyof typeof gameInput] = true;
      }
    };
    const handleKeyUp = (e: KeyboardEvent) => {
      let key = e.key.toLowerCase();
      if (key === ' ') key = 'fire';
      if (key === 'b') key = 'blaster';
      if (key in gameInput) {
        gameInput[key as keyof typeof gameInput] = false;
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('keyup', handleKeyUp);
    };
  }, []);
}
