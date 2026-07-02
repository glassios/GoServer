import { useEffect, useRef, useState } from 'react';
import type { TurretDefinition } from '@/src/shared/types/turret';
import type { ShipWeaponSlot } from '@/src/shared/types/weapon';
import { centerAim } from '@/src/shared/turret/turretRotation';
import { loadTurretDefinition } from '@/src/shared/turret/loadTurret';
import { ShipWeaponMount } from '@/src/shared/ship/ShipWeaponMount';
import type { FiringInput } from '@/src/shared/weapon/resolveFiring';

interface MountedTurretProps {
  slot: ShipWeaponSlot;
  animate: boolean;
  /** @deprecated use `firing` */
  previewFire?: boolean;
  firing?: FiringInput;
}

export function MountedTurret({
  slot,
  animate,
  previewFire = false,
  firing,
}: MountedTurretProps) {
  const [turret, setTurret] = useState<TurretDefinition | null>(null);
  const [aimAngle, setAimAngle] = useState(0);
  const aimDir = useRef(1);
  const firingInput: FiringInput = firing ?? previewFire;

  useEffect(() => {
    let cancelled = false;
    void loadTurretDefinition(slot.turretAsset).then((def) => {
      if (!cancelled) {
        setTurret(def);
        setAimAngle(centerAim(def.rotation));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [slot.turretAsset]);

  useEffect(() => {
    if (!turret) return;
    const { minAngle, maxAngle } = turret.rotation;
    setAimAngle((a) => Math.min(maxAngle, Math.max(minAngle, a)));
  }, [turret?.rotation.minAngle, turret?.rotation.maxAngle, turret]);

  useEffect(() => {
    if (!animate || !turret) return;

    let raf = 0;
    let last = performance.now();

    const tick = (now: number) => {
      const delta = Math.min((now - last) / 1000, 0.05);
      last = now;
      const { minAngle, maxAngle, turnSpeed } = turret.rotation;

      setAimAngle((a) => {
        let next = a + aimDir.current * turnSpeed * delta;
        if (next >= maxAngle) {
          next = maxAngle;
          aimDir.current = -1;
        } else if (next <= minAngle) {
          next = minAngle;
          aimDir.current = 1;
        }
        return next;
      });

      raf = requestAnimationFrame(tick);
    };

    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [animate, turret]);

  if (!turret) return null;

  return (
    <ShipWeaponMount
      slot={slot}
      definition={turret}
      aimAngle={aimAngle}
      firing={firingInput}
      showEditorGizmos
    />
  );
}
