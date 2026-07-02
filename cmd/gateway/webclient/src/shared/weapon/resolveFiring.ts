/** Boolean or live getter (game loop reads input without React re-renders). */
export type FiringInput = boolean | (() => boolean);

export function isFiring(input: FiringInput): boolean {
  return typeof input === 'function' ? input() : input;
}
