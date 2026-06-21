package systems

import (
	"context"
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/persistence"
)

// End-to-end Phase 0 check: the code catalog, the in-memory ShipRepository and BakeShip all
// agree — every stock loadout bakes into a valid set of battle-ready ECS components without a DB.
func TestBakeShip_AllStockLoadouts(t *testing.T) {
	repo := persistence.NewInMemoryShipRepository()
	ctx := context.Background()

	var nextID domain.EntityID = 1000
	for _, hull := range domain.StockHulls {
		cfg := domain.DefaultLoadoutForShipType(hull.HullID)
		cfg.OwnerType = "npc"
		entityID := nextID
		nextID++

		world := ecs.NewWorld()
		if err := BakeShip(world, entityID, cfg, repo, ctx); err != nil {
			t.Fatalf("BakeShip(%s) failed: %v", hull.HullID, err)
		}

		// Health
		hVal, ok := world.GetComponent(entityID, domain.Health{})
		if !ok || hVal.(*domain.Health).Max <= 0 {
			t.Errorf("%s: missing/zero Health", hull.HullID)
		}

		// ShipConfig (movement)
		cfgVal, ok := world.GetComponent(entityID, domain.ShipConfig{})
		if !ok || cfgVal.(*domain.ShipConfig).MaxSpeed <= 0 {
			t.Errorf("%s: missing/zero MaxSpeed", hull.HullID)
		}

		// Flux
		fVal, ok := world.GetComponent(entityID, domain.FluxState{})
		if !ok || fVal.(*domain.FluxState).Capacity <= 0 {
			t.Errorf("%s: missing/zero FluxState capacity", hull.HullID)
		}

		// Weapon group must have one fitted weapon per slot.
		wgVal, ok := world.GetComponent(entityID, domain.WeaponGroup{})
		if !ok {
			t.Fatalf("%s: missing WeaponGroup", hull.HullID)
		}
		wg := wgVal.(*domain.WeaponGroup)
		if len(wg.Weapons) != len(hull.WeaponSlots) {
			t.Errorf("%s: %d baked weapons, want %d", hull.HullID, len(wg.Weapons), len(hull.WeaponSlots))
		}
		for _, w := range wg.Weapons {
			if w.Definition.DamagePerShot <= 0 {
				t.Errorf("%s: weapon %s has no damage", hull.HullID, w.SlotID)
			}
		}

		// Shield only for hulls that have one.
		_, hasShield := world.GetComponent(entityID, domain.Shield{})
		if hull.ShieldType != "none" && !hasShield {
			t.Errorf("%s: expected a Shield component", hull.HullID)
		}
	}
}

// Hullmods must actually modify baked stats (armor/speed multipliers applied).
func TestBakeShip_HullmodsApply(t *testing.T) {
	repo := persistence.NewInMemoryShipRepository()
	ctx := context.Background()

	base := domain.DefaultLoadoutForShipType("destroyer")
	modded := domain.DefaultLoadoutForShipType("destroyer")
	modded.FittedHullmods = []string{domain.HullmodAugmentedEngines}

	wBase := ecs.NewWorld()
	wMod := ecs.NewWorld()
	if err := BakeShip(wBase, 1, base, repo, ctx); err != nil {
		t.Fatalf("bake base: %v", err)
	}
	if err := BakeShip(wMod, 2, modded, repo, ctx); err != nil {
		t.Fatalf("bake modded: %v", err)
	}

	baseSpeed := mustSpeed(t, wBase, 1)
	modSpeed := mustSpeed(t, wMod, 2)
	if !(modSpeed > baseSpeed) {
		t.Errorf("augmented engines did not raise speed: base=%v mod=%v", baseSpeed, modSpeed)
	}
}

func mustSpeed(t *testing.T, world *ecs.World, id domain.EntityID) float32 {
	t.Helper()
	cfgVal, ok := world.GetComponent(id, domain.ShipConfig{})
	if !ok {
		t.Fatalf("entity %d missing ShipConfig", id)
	}
	return cfgVal.(*domain.ShipConfig).MaxSpeed
}
