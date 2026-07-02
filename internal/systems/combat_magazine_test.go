package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// A magazine weapon (B5) fires its Magazine rounds then goes silent for ReloadTime before it can
// fire again — a real burst-reload rhythm rather than a flat cooldown.
func TestMagazine_FiresBurstThenReloads(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	attacker := domain.EntityID(1)
	target := domain.EntityID(2)

	world.RegisterEntityWithID(attacker, domain.EntityNPC)
	world.AddComponent(attacker, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(attacker, &domain.CombatTeam{TeamID: 1})
	def := domain.WeaponDefinition{DamagePerShot: 5, DamageType: domain.DamageKinetic, Range: 1000, Cooldown: 0, Magazine: 3, ReloadTime: 1.0}
	world.AddComponent(attacker, &domain.WeaponGroup{Weapons: []domain.FittedWeaponState{{Definition: def, Ammo: 3}}})
	world.AddComponent(attacker, &domain.Weapon{Active: true, TargetID: target, Range: 1000})

	world.RegisterEntityWithID(target, domain.EntityNPC)
	world.AddComponent(target, &domain.Transform{X: 10, Y: 0})
	world.AddComponent(target, &domain.Health{Current: 1000, Max: 1000})
	world.AddComponent(target, &domain.CombatFx{})

	hp := func() int32 {
		v, _ := world.GetComponent(target, domain.Health{})
		return v.(*domain.Health).Current
	}

	// Empty the magazine: 3 shots × 5 dmg (kinetic ×1 on bare hull) = 15.
	for i := 0; i < 3; i++ {
		cs.fire(world, attacker, 0.05)
	}
	if dmg := 1000 - hp(); dmg != 15 {
		t.Fatalf("expected 3 shots (15 dmg) to empty the magazine, got %d", dmg)
	}

	// Immediately after: reloading, must not fire.
	cs.fire(world, attacker, 0.05)
	if 1000-hp() != 15 {
		t.Errorf("weapon should be silent while reloading, dmg=%d", 1000-hp())
	}

	// Run out the ~1 s reload and a few shots beyond: it refills and resumes firing.
	for i := 0; i < 25; i++ {
		cs.fire(world, attacker, 0.05)
	}
	if 1000-hp() <= 15 {
		t.Errorf("weapon should resume firing after reload, total dmg=%d", 1000-hp())
	}
}
