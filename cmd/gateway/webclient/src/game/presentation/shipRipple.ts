export const RIPPLE_MAX_HITS = 10;

export interface RippleHit {
  x: number;
  y: number;
  time: number;
  scale: number;
}

export function appendRippleHit(hits: RippleHit[], hit: Omit<RippleHit, 'time'> & { time?: number }): void {
  hits.push({
    x: hit.x,
    y: hit.y,
    time: hit.time ?? performance.now(),
    scale: hit.scale,
  });
  if (hits.length > RIPPLE_MAX_HITS) hits.shift();
}
