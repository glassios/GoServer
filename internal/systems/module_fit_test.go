package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
)

// Cruiser hull (id 8) WS2 is a MEDIUM ENERGY slot — fits the Heavy Blaster module.
func cfgWith(slotWeapon map[string]string) *domain.ShipConfiguration {
	return &domain.ShipConfiguration{HullID: 8, FittedWeapons: slotWeapon}
}

func TestApplyFitInventory_ConsumesModuleOnFit(t *testing.T) {
	cargo := &domain.Cargo{Capacity: 100}
	cargo.AddResourceTypeQuantity("Heavy Blaster", 1)

	current := cfgWith(map[string]string{})                                       // nothing fitted
	requested := cfgWith(map[string]string{"WS2": domain.WeaponHeavyBlaster})     // fit one module

	if err := ApplyFitInventory(cargo, current, requested); err != nil {
		t.Fatalf("expected fit to succeed, got %v", err)
	}
	if got := cargo.GetResourceTypeQuantity("Heavy Blaster"); got != 0 {
		t.Fatalf("expected module consumed (0 left), got %d", got)
	}
}

func TestApplyFitInventory_ReturnsModuleOnUnfit(t *testing.T) {
	cargo := &domain.Cargo{Capacity: 100}
	current := cfgWith(map[string]string{"WS2": domain.WeaponHeavyBlaster})
	requested := cfgWith(map[string]string{}) // remove it

	if err := ApplyFitInventory(cargo, current, requested); err != nil {
		t.Fatalf("unfit should not error, got %v", err)
	}
	if got := cargo.GetResourceTypeQuantity("Heavy Blaster"); got != 1 {
		t.Fatalf("expected module returned (1), got %d", got)
	}
}

func TestApplyFitInventory_RejectsWithoutModule(t *testing.T) {
	cargo := &domain.Cargo{Capacity: 100} // no modules owned
	current := cfgWith(map[string]string{})
	requested := cfgWith(map[string]string{"WS2": domain.WeaponHeavyBlaster})

	if err := ApplyFitInventory(cargo, current, requested); err == nil {
		t.Fatal("expected rejection when module not owned")
	}
}

func TestApplyFitInventory_BasicWeaponsFree(t *testing.T) {
	cargo := &domain.Cargo{Capacity: 100} // empty
	current := cfgWith(map[string]string{})
	requested := cfgWith(map[string]string{"WS2": domain.WeaponLightLaser}) // basic, no module

	if err := ApplyFitInventory(cargo, current, requested); err != nil {
		t.Fatalf("basic weapon should not require a module, got %v", err)
	}
}

func TestApplyFitInventory_SwapSameModuleNoNetChange(t *testing.T) {
	cargo := &domain.Cargo{Capacity: 100}
	// Move the same module from WS2 to WS3 — net module count unchanged, no cargo needed.
	current := cfgWith(map[string]string{"WS2": domain.WeaponHeavyBlaster})
	requested := cfgWith(map[string]string{"WS3": domain.WeaponHeavyBlaster})

	if err := ApplyFitInventory(cargo, current, requested); err != nil {
		t.Fatalf("relocating same module should be free, got %v", err)
	}
	if got := cargo.GetResourceTypeQuantity("Heavy Blaster"); got != 0 {
		t.Fatalf("expected no cargo change, got %d", got)
	}
}
