// App-wide singletons: one snapshot store fed by one gateway connection.
import { GameConnection } from './connection';
import { SnapshotStore } from './snapshotStore';

export const store = new SnapshotStore();
export const connection = new GameConnection(store);
