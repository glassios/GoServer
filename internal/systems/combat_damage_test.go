package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// helper: a target with full shield/armor/hull and flux headroom.
func newDamageTarget(world *ecs.World, id domain.EntityID, shieldDown bool) {
	world.RegisterEntityWithID(id, domain.EntityNPC)
	world.AddComponent(id, &domain.Health{Current: 100, Max: 100})
	world.AddComponent(id, &domain.Shield{Current: 100, Max: 100, Efficiency: 1.0, Type: "omni", Down: shieldDown})
	world.AddComponent(id, &domain.ArmorGrid{Current: 50, Max: 50})
	world.AddComponent(id, &domain.FluxState{Current: 0, Capacity: 200})
	world.AddComponent(id, &domain.CombatFx{})
}

func TestApplyDamage_KineticHitsShieldAndRaisesFlux(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	id := domain.EntityID(1)
	newDamageTarget(world, id, false)

	// Kinetic ×2 vs shields: 40 dmg → 80 shield damage, 80 flux, hull/armor untouched.
	killed := cs.applyDamage(world, id, 40, domain.DamageKinetic)
	if killed {
		t.Fatal("should not kill through full shield")
	}
	sh := mustGet[*domain.Shield](t, world, id)
	if sh.Current != 20 {
		t.Errorf("shield: got %d, want 20", sh.Current)
	}
	fl := mustGet[*domain.FluxState](t, world, id)
	if fl.Current != 80 {
		t.Errorf("flux: got %v, want 80", fl.Current)
	}
	h := mustGet[*domain.Health](t, world, id)
	ar := mustGet[*domain.ArmorGrid](t, world, id)
	if h.Current != 100 || ar.Current != 50 {
		t.Errorf("hull/armor should be untouched, got hull=%d armor=%v", h.Current, ar.Current)
	}
}

func TestApplyDamage_ShieldDownExplosiveChewsArmorThenHull(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	id := domain.EntityID(2)
	newDamageTarget(world, id, true) // shield down

	// Explosive ×2 vs armor: 30 dmg → 60 armor damage. Armor=50 breaks (absorbs 50/60),
	// remaining 30*(1-0.833)=5 → hull -5 → 95.
	cs.applyDamage(world, id, 30, domain.DamageExplosive)
	ar := mustGet[*domain.ArmorGrid](t, world, id)
	if ar.Current != 0 {
		t.Errorf("armor should be depleted, got %v", ar.Current)
	}
	h := mustGet[*domain.Health](t, world, id)
	if h.Current != 95 {
		t.Errorf("hull: got %d, want 95", h.Current)
	}
}

func TestApplyDamage_FluxOverloadDropsShield(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	id := domain.EntityID(3)
	world.RegisterEntityWithID(id, domain.EntityNPC)
	world.AddComponent(id, &domain.Health{Current: 100, Max: 100})
	world.AddComponent(id, &domain.Shield{Current: 1000, Max: 1000, Efficiency: 1.0, Type: "omni"})
	world.AddComponent(id, &domain.ArmorGrid{Current: 50, Max: 50})
	world.AddComponent(id, &domain.FluxState{Current: 0, Capacity: 100}) // small flux pool
	world.AddComponent(id, &domain.CombatFx{})

	// Energy ×1 vs shields: 120 dmg → 120 flux absorbed, exceeds capacity 100 → overload.
	cs.applyDamage(world, id, 120, domain.DamageEnergy)
	fl := mustGet[*domain.FluxState](t, world, id)
	if !fl.Overloaded {
		t.Errorf("expected overload when absorbed flux exceeds capacity, flux=%v", fl.Current)
	}
}

func mustGet[T any](t *testing.T, world *ecs.World, id domain.EntityID) T {
	t.Helper()
	var key any
	var zero T
	switch any(zero).(type) {
	case *domain.Health:
		key = domain.Health{}
	case *domain.Shield:
		key = domain.Shield{}
	case *domain.ArmorGrid:
		key = domain.ArmorGrid{}
	case *domain.FluxState:
		key = domain.FluxState{}
	default:
		t.Fatalf("mustGet: unsupported type")
	}
	v, ok := world.GetComponent(id, key)
	if !ok {
		t.Fatalf("component not found")
	}
	return v.(T)
}
