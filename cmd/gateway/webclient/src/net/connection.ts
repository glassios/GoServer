// WebSocket connection to the gateway (/ws), shared with the legacy client's protocol.
// Auto-authenticates on open, locks onto the player's system, and feeds the SnapshotStore.
import type { SnapshotStore } from './snapshotStore';
import type { WorldSnapshot } from './types';

export type ConnectionStatus = 'connecting' | 'open' | 'authed' | 'closed';

function makePilotName(): string {
  const existing = sessionStorage.getItem('client3d_pilot');
  if (existing) return existing;
  const name = `Pilot3D-${Math.floor(Math.random() * 1e6).toString(36)}`;
  sessionStorage.setItem('client3d_pilot', name);
  return name;
}

export class GameConnection {
  private socket: WebSocket | null = null;
  private readonly store: SnapshotStore;
  private systemLocked = false;
  readonly pilotName = makePilotName();

  status: ConnectionStatus = 'closed';
  onStatus?: (s: ConnectionStatus) => void;
  onAuth?: (entityId: string) => void;

  constructor(store: SnapshotStore) {
    this.store = store;
  }

  private setStatus(s: ConnectionStatus): void {
    this.status = s;
    this.onStatus?.(s);
  }

  connect(): void {
    if (this.socket) return;
    // /ws is served at the gateway root regardless of the client's /client3d/ base path.
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${window.location.host}/ws`;
    this.setStatus('connecting');
    const socket = new WebSocket(url);
    this.socket = socket;

    socket.onopen = () => {
      this.setStatus('open');
      socket.send(JSON.stringify({ action: 'auth', login: this.pilotName }));
    };
    socket.onclose = () => {
      this.setStatus('closed');
      this.socket = null;
    };
    socket.onerror = () => this.setStatus('closed');
    socket.onmessage = (ev) => this.handleMessage(ev.data);
  }

  private handleMessage(data: unknown): void {
    if (typeof data !== 'string') return;
    let msg: {
      type?: string;
      system_id?: number;
      snapshot?: WorldSnapshot;
      data?: { success?: boolean; entity_id?: string };
    };
    try {
      msg = JSON.parse(data);
    } catch {
      return;
    }

    if (msg.snapshot && typeof msg.system_id === 'number') {
      const entities = msg.snapshot.entities ?? [];
      const me = this.store.localPlayerId;
      const containsMe = !!me && entities.some((e) => e.entity_id === me);
      if (containsMe) {
        this.store.setSystem(msg.system_id);
        this.systemLocked = true;
      }
      // Before we've located ourselves, default to system 1; afterwards, only our system.
      if (msg.system_id === this.store.systemId || (!this.systemLocked && msg.system_id === 1)) {
        this.store.ingest(entities, performance.now());
      }
      return;
    }

    if (msg.type === 'auth_response' && msg.data?.success) {
      this.store.localPlayerId = msg.data.entity_id;
      this.systemLocked = false;
      this.setStatus('authed');
      this.onAuth?.(msg.data.entity_id);
      return;
    }

    if (msg.type === 'system_transition' && typeof msg.system_id === 'number') {
      this.store.setSystem(msg.system_id);
      this.systemLocked = true;
    }
  }

  sendMove(x: number, y: number): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify({ action: 'move', x, y }));
    }
  }

  sendShoot(active: boolean, targetId?: string): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify({ action: 'shoot', active, target_id: targetId ?? '0' }));
    }
  }

  /** Join a battle instance. `instanceId` = the CombatMarker's target_id; alignFleetId 0 = FFA. */
  sendJoinCombat(instanceId: string, alignFleetId = 0): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(
        JSON.stringify({ action: 'join_combat', target_id: instanceId, align_with_fleet_id: alignFleetId })
      );
    }
  }

  /** Assign a fleet ship's combat role/strategy (set_fleet_tactics). */
  sendFleetTactics(shipId: number, role: string, strategy: string): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify({ action: 'set_fleet_tactics', ship_id: shipId, role, strategy }));
    }
  }

  disconnect(): void {
    this.socket?.close();
    this.socket = null;
  }
}
