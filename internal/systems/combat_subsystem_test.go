package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// A missile that penetrates to armor/hull should knock out the engines first, then the weapons on
// a second strike. Non-missile weapons never touch subsystems.
func TestSubsystem_MissileHitDisablesEnginesThenWeapons(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	id := domain.EntityID(1)
	world.RegisterEntityWithID(id, domain.EntityNPC)
	world.AddComponent(id, &domain.Health{Current: 1000, Max: 1000})
	world.AddComponent(id, &domain.ArmorGrid{Current: 1000, Max: 1000}) // soak damage, survive hits
	world.AddComponent(id, &domain.FluxState{Current: 0, Capacity: 500})
	world.AddComponent(id, &domain.CombatFx{})
	world.AddComponent(id, &domain.SubsystemState{})

	// A kinetic (non-missile) hit must not disable any subsystem.
	cs.applyDamage(world, id, 0, 0, 20, domain.DamageKinetic, "")
	ss := mustGetSub(t, world, id)
	if ss.EngineHitTimer != 0 || ss.WeaponHitTimer != 0 {
		t.Fatalf("non-missile hit disabled a subsystem: %+v", *ss)
	}

	// First missile hit: engines go down, weapons still up.
	cs.applyDamage(world, id, 0, 0, 20, domain.DamageExplosive, domain.WeaponClassMissile)
	if ss.EngineHitTimer <= 0 {
		t.Errorf("first missile hit should disable engines, got %+v", *ss)
	}
	if ss.WeaponHitTimer != 0 {
		t.Errorf("first missile hit should not yet disable weapons, got %+v", *ss)
	}

	// Second missile hit (engines still down): weapons go down too.
	cs.applyDamage(world, id, 0, 0, 20, domain.DamageExplosive, domain.WeaponClassMissile)
	if ss.WeaponHitTimer <= 0 {
		t.Errorf("second missile hit should disable weapons, got %+v", *ss)
	}
}

// Subsystem timers decay to zero over time via CombatSystem.Update.
func TestSubsystem_TimersDecay(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	id := domain.EntityID(2)
	world.RegisterEntityWithID(id, domain.EntityNPC)
	world.AddComponent(id, &domain.SubsystemState{EngineHitTimer: 0.1, WeaponHitTimer: 0.1})

	// One 0.05s tick: still active but reduced.
	cs.Update(world, 0.05)
	ss := mustGetSub(t, world, id)
	if ss.EngineHitTimer <= 0 || ss.EngineHitTimer > 0.06 {
		t.Errorf("engine timer should have decayed to ~0.05, got %v", ss.EngineHitTimer)
	}

	// Two more ticks: fully worn off, clamped at 0.
	cs.Update(world, 0.05)
	cs.Update(world, 0.05)
	if ss.EngineHitTimer != 0 || ss.WeaponHitTimer != 0 {
		t.Errorf("timers should clamp to 0, got %+v", *ss)
	}
}

func mustGetSub(t *testing.T, world *ecs.World, id domain.EntityID) *domain.SubsystemState {
	t.Helper()
	v, ok := world.GetComponent(id, domain.SubsystemState{})
	if !ok {
		t.Fatal("SubsystemState not found")
	}
	return v.(*domain.SubsystemState)
}
