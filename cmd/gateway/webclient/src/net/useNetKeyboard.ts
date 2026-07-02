import { useEffect } from 'react';
import { connection } from './session';

/** Max velocity command magnitude per axis — matches the legacy client's `force`. */
const FORCE = 120.0;

// WASD → server velocity vector. Identity render mapping (+Y up) means W → +Y, the opposite
// sign from the legacy Y-down client. Sends only on change to avoid flooding the socket.
// `enabled` is false in battle mode (ships are AI-controlled; the player is an observer).
export function useNetKeyboard(enabled = true): void {
  useEffect(() => {
    if (!enabled) return;
    const keys = { w: false, a: false, s: false, d: false };
    let lastX = 0;
    let lastY = 0;

    const send = () => {
      let vx = 0;
      let vy = 0;
      if (keys.w) vy += FORCE;
      if (keys.s) vy -= FORCE;
      if (keys.a) vx -= FORCE;
      if (keys.d) vx += FORCE;
      if (vx !== lastX || vy !== lastY) {
        connection.sendMove(vx, vy);
        lastX = vx;
        lastY = vy;
      }
    };

    const onKey = (down: boolean) => (e: KeyboardEvent) => {
      const k = e.key.toLowerCase();
      if (k === 'w' || k === 'a' || k === 's' || k === 'd') {
        keys[k as keyof typeof keys] = down;
        send();
      }
    };
    const kd = onKey(true);
    const ku = onKey(false);
    window.addEventListener('keydown', kd);
    window.addEventListener('keyup', ku);
    return () => {
      window.removeEventListener('keydown', kd);
      window.removeEventListener('keyup', ku);
    };
  }, [enabled]);
}
