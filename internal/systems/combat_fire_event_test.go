package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// buildShooter wires an attacker that will fire one mount at a target this tick.
func buildShooter(world *ecs.World, attackerID, targetID domain.EntityID, def domain.WeaponDefinition) {
	world.RegisterEntityWithID(attackerID, domain.EntityNPC)
	world.AddComponent(attackerID, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(attackerID, &domain.CombatTeam{TeamID: 1})
	world.AddComponent(attackerID, &domain.FluxState{Current: 0, Capacity: 500, DissipationRate: 10})
	world.AddComponent(attackerID, &domain.CombatFx{})
	world.AddComponent(attackerID, &domain.Weapon{Active: true, TargetID: targetID})
	world.AddComponent(attackerID, &domain.WeaponGroup{Weapons: []domain.FittedWeaponState{
		{SlotID: "s1", Definition: def, Cooldown: 0},
	}})

	world.RegisterEntityWithID(targetID, domain.EntityNPC)
	world.AddComponent(targetID, &domain.Transform{X: 100, Y: 0})
	world.AddComponent(targetID, &domain.CombatTeam{TeamID: 2})
	world.AddComponent(targetID, &domain.Health{Current: 1000, Max: 1000})
	world.AddComponent(targetID, &domain.CombatFx{})
}

func TestFire_ProjectileWeaponEmitsFireEvent(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	def := domain.WeaponDefinition{
		DamagePerShot: 10, DamageType: domain.DamageKinetic, Range: 500, Cooldown: 0.5,
		FluxCost: 8, Class: domain.WeaponClassProjectile, ProjectileSpeed: 900,
	}
	buildShooter(world, 1, 2, def)

	cs.Update(world, 0.05)

	if len(cs.Fires) != 1 {
		t.Fatalf("projectile shot should emit exactly one fire event, got %d", len(cs.Fires))
	}
	f := cs.Fires[0]
	if f.AttackerID != 1 || f.TargetID != 2 {
		t.Errorf("fire event ids: got attacker=%d target=%d, want 1/2", f.AttackerID, f.TargetID)
	}
	if f.OriginX != 0 || f.TargetX != 100 {
		t.Errorf("fire event geometry: origin=(%v,%v) target=(%v,%v)", f.OriginX, f.OriginY, f.TargetX, f.TargetY)
	}
	if f.Speed != 900 || f.WeaponClass != domain.WeaponClassProjectile || f.DamageType != domain.DamageKinetic {
		t.Errorf("fire event payload: got speed=%v class=%q type=%q", f.Speed, f.WeaponClass, f.DamageType)
	}
}

func TestFire_MissileWeaponEmitsFireEvent(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	def := domain.WeaponDefinition{
		DamagePerShot: 20, DamageType: domain.DamageExplosive, Range: 600, Cooldown: 2.0,
		FluxCost: 0, Class: domain.WeaponClassMissile, ProjectileSpeed: 300,
	}
	buildShooter(world, 1, 2, def)

	cs.Update(world, 0.05)

	if len(cs.Fires) != 1 {
		t.Fatalf("missile shot should emit one fire event, got %d", len(cs.Fires))
	}
	if cs.Fires[0].WeaponClass != domain.WeaponClassMissile {
		t.Errorf("weapon class: got %q, want missile", cs.Fires[0].WeaponClass)
	}
}

func TestFire_HitscanWeaponEmitsNoFireEvent(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	def := domain.WeaponDefinition{
		DamagePerShot: 8, DamageType: domain.DamageEnergy, Range: 450, Cooldown: 0.4, FluxCost: 6,
		// Class empty → instant hitscan, no traveling bolt.
	}
	buildShooter(world, 1, 2, def)

	cs.Update(world, 0.05)

	if len(cs.Fires) != 0 {
		t.Fatalf("hitscan shot must not emit a fire event, got %d", len(cs.Fires))
	}
	// Sanity: it still dealt damage (proves the mount actually fired).
	h := mustGet[*domain.Health](t, world, 2)
	if h.Current >= 1000 {
		t.Errorf("hitscan should have damaged the target, hp=%d", h.Current)
	}
}

// Fires must reset each tick so a snapshot never re-sends last tick's bolts.
func TestFire_FiresResetEachTick(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	def := domain.WeaponDefinition{
		DamagePerShot: 10, DamageType: domain.DamageKinetic, Range: 500, Cooldown: 0, // cooldown 0 → fires every tick
		FluxCost: 1, Class: domain.WeaponClassProjectile, ProjectileSpeed: 900,
	}
	buildShooter(world, 1, 2, def)

	cs.Update(world, 0.05)
	first := len(cs.Fires)
	cs.Update(world, 0.05)
	second := len(cs.Fires)
	if first == 0 || second == 0 {
		t.Fatalf("expected fires on both ticks, got %d then %d", first, second)
	}
	if second > first {
		t.Errorf("Fires should reset each tick, not accumulate: %d then %d", first, second)
	}
}
