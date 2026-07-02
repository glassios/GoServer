import { useEffect, useState } from 'react';
import { store } from './session';

/** Current system id, re-rendering when it changes (open world ↔ battle instance). */
export function useSystemId(): number {
  const [sys, setSys] = useState(store.systemId);
  useEffect(() => store.onSystemChange(setSys), []);
  return sys;
}

/** Live list of entity ids in the store, re-rendering on spawn/despawn. */
export function useEntityIds(): string[] {
  const [ids, setIds] = useState<string[]>(() => [...store.entities.keys()]);
  useEffect(() => {
    const refresh = () => setIds([...store.entities.keys()]);
    const offSpawn = store.onSpawn(refresh);
    const offDespawn = store.onDespawn(refresh);
    refresh();
    return () => {
      offSpawn();
      offDespawn();
    };
  }, []);
  return ids;
}
