// Holds the latest server state for every entity, plus a short pose history per entity for
// smooth interpolation. Snapshots arrive at the server tick rate (~20/s); we render slightly
// in the past (INTERP_DELAY_MS) and lerp between the two bracketing samples.
//
// This is a plain singleton (not React state) so the render loop can read it every frame in
// useFrame without triggering React re-renders. Components subscribe to spawn/despawn events.
import { SPACE_WORLD_SCALE, worldScaleFor } from './coords';
import type { RawEntitySnapshot } from './types';

/** Render ~2 ticks in the past so we always interpolate between two known samples. */
export const INTERP_DELAY_MS = 100;

interface PoseSample {
  t: number; // client receive time (ms, performance.now())
  x: number;
  y: number;
  rot: number;
  vx: number;
  vy: number;
}

/** Interpolated render-space pose for one entity at the current render time. */
export interface RenderPose {
  x: number;
  y: number;
  rotation: number;
  speed: number; // server-space speed magnitude (for engine plume gating)
}

export interface NetEntity {
  id: string;
  entityType: number;
  shipType: string;
  /** Latest raw fields (gameplay state: hp/shield/armor/flux/firing/etc.). */
  state: RawEntitySnapshot;
  /** Previous raw fields, for delta detection (damage, fire events) in later slices. */
  prev: RawEntitySnapshot | null;
  buffer: PoseSample[];
  lastSeen: number;
}

const MAX_POSE_SAMPLES = 12;
/** Drop entities not seen for this long (server stopped sending them). */
const DESPAWN_GRACE_MS = 1500;

function shortestAngleLerp(a: number, b: number, t: number): number {
  let diff = (b - a) % (Math.PI * 2);
  if (diff > Math.PI) diff -= Math.PI * 2;
  if (diff < -Math.PI) diff += Math.PI * 2;
  return a + diff * t;
}

export class SnapshotStore {
  readonly entities = new Map<string, NetEntity>();
  localPlayerId: string | null = null;
  systemId = 1;
  /** Server→render distance scale for the current system (open world vs battle arena). */
  worldScale = SPACE_WORLD_SCALE;

  private spawnListeners = new Set<(e: NetEntity) => void>();
  private despawnListeners = new Set<(id: string) => void>();
  private systemListeners = new Set<(systemId: number) => void>();

  onSpawn(fn: (e: NetEntity) => void): () => void {
    this.spawnListeners.add(fn);
    return () => this.spawnListeners.delete(fn);
  }

  onDespawn(fn: (id: string) => void): () => void {
    this.despawnListeners.add(fn);
    return () => this.despawnListeners.delete(fn);
  }

  /** Notified when the player's current system changes (open world ↔ battle instance). */
  onSystemChange(fn: (systemId: number) => void): () => void {
    this.systemListeners.add(fn);
    return () => this.systemListeners.delete(fn);
  }

  /** Switch the active system. No-op if unchanged; otherwise clears all entities (they belong
   *  to the old system/instance) and notifies despawn + system-change listeners. */
  setSystem(systemId: number): void {
    if (systemId === this.systemId) return;
    for (const id of [...this.entities.keys()]) {
      this.entities.delete(id);
      this.despawnListeners.forEach((fn) => fn(id));
    }
    this.systemId = systemId;
    this.worldScale = worldScaleFor(systemId);
    this.systemListeners.forEach((fn) => fn(systemId));
  }

  /** Ingest one world snapshot's entities. `now` is performance.now() at receive time. */
  ingest(rawEntities: RawEntitySnapshot[], now: number): void {
    for (const raw of rawEntities) {
      let ent = this.entities.get(raw.entity_id);
      if (!ent) {
        ent = {
          id: raw.entity_id,
          entityType: raw.entity_type,
          shipType: raw.ship_type,
          state: raw,
          prev: null,
          buffer: [],
          lastSeen: now,
        };
        this.entities.set(raw.entity_id, ent);
        this.spawnListeners.forEach((fn) => fn(ent!));
      } else {
        ent.prev = ent.state;
        ent.state = raw;
        ent.shipType = raw.ship_type;
        ent.entityType = raw.entity_type;
      }
      ent.lastSeen = now;
      ent.buffer.push({ t: now, x: raw.x, y: raw.y, rot: raw.rotation, vx: raw.vx, vy: raw.vy });
      if (ent.buffer.length > MAX_POSE_SAMPLES) ent.buffer.shift();
    }

    // Despawn entities not seen recently.
    for (const [id, ent] of this.entities) {
      if (now - ent.lastSeen > DESPAWN_GRACE_MS) {
        this.entities.delete(id);
        this.despawnListeners.forEach((fn) => fn(id));
      }
    }
  }

  /** Sample an entity's interpolated render-space pose at `renderTime = now - INTERP_DELAY_MS`. */
  sampleRender(id: string, now: number): RenderPose | null {
    const ent = this.entities.get(id);
    if (!ent || ent.buffer.length === 0) return null;
    const buf = ent.buffer;
    const renderTime = now - INTERP_DELAY_MS;

    // Find the two samples bracketing renderTime.
    let a = buf[0];
    let b = buf[buf.length - 1];
    for (let i = 0; i < buf.length - 1; i++) {
      if (buf[i].t <= renderTime && buf[i + 1].t >= renderTime) {
        a = buf[i];
        b = buf[i + 1];
        break;
      }
    }

    let sx: number;
    let sy: number;
    let srot: number;
    if (renderTime <= a.t) {
      sx = a.x;
      sy = a.y;
      srot = a.rot;
    } else if (renderTime >= b.t || b.t === a.t) {
      // Past the newest sample: briefly extrapolate with last known velocity.
      const dt = Math.min((renderTime - b.t) / 1000, 0.25);
      sx = b.x + b.vx * dt;
      sy = b.y + b.vy * dt;
      srot = b.rot;
    } else {
      const f = (renderTime - a.t) / (b.t - a.t);
      sx = a.x + (b.x - a.x) * f;
      sy = a.y + (b.y - a.y) * f;
      srot = shortestAngleLerp(a.rot, b.rot, f);
    }

    const speed = Math.hypot(ent.state.vx, ent.state.vy);
    return { x: sx * this.worldScale, y: sy * this.worldScale, rotation: srot, speed };
  }
}
