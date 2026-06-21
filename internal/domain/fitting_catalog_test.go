package domain

import "testing"

// Every hull in the catalog must have a unique numeric id and string id, and the lookup
// indexes must agree with the slices.
func TestStockCatalog_Indexes(t *testing.T) {
	seenNum := map[uint32]bool{}
	seenStr := map[string]bool{}
	for _, h := range StockHulls {
		if seenNum[h.ID] {
			t.Errorf("duplicate hull numeric id: %d", h.ID)
		}
		if seenStr[h.HullID] {
			t.Errorf("duplicate hull string id: %s", h.HullID)
		}
		seenNum[h.ID] = true
		seenStr[h.HullID] = true

		if HullByNumericID(h.ID) == nil {
			t.Errorf("HullByNumericID(%d) returned nil", h.ID)
		}
		if HullByStringID(h.HullID) == nil {
			t.Errorf("HullByStringID(%q) returned nil", h.HullID)
		}
		if len(h.WeaponSlots) == 0 {
			t.Errorf("hull %q has no weapon slots", h.HullID)
		}
	}

	for _, w := range StockWeapons {
		if WeaponByID(w.WeaponID) == nil {
			t.Errorf("WeaponByID(%q) returned nil", w.WeaponID)
		}
	}
	for _, m := range StockHullmods {
		if HullmodByID(m.ModID) == nil {
			t.Errorf("HullmodByID(%q) returned nil", m.ModID)
		}
	}
}

// DefaultLoadoutForShipType must produce a valid, fully-fitted configuration for every
// stock hull, and resolvable weapons for every fitted slot.
func TestDefaultLoadout_FitsEveryHull(t *testing.T) {
	for _, h := range StockHulls {
		cfg := DefaultLoadoutForShipType(h.HullID)
		if cfg.HullID != h.ID {
			t.Errorf("hull %q: loadout HullID = %d, want %d", h.HullID, cfg.HullID, h.ID)
		}
		if len(cfg.FittedWeapons) != len(h.WeaponSlots) {
			t.Errorf("hull %q: fitted %d weapons, hull has %d slots", h.HullID, len(cfg.FittedWeapons), len(h.WeaponSlots))
		}
		for _, slot := range h.WeaponSlots {
			weaponID, ok := cfg.FittedWeapons[slot.SlotID]
			if !ok {
				t.Errorf("hull %q: slot %s not fitted", h.HullID, slot.SlotID)
				continue
			}
			if WeaponByID(weaponID) == nil {
				t.Errorf("hull %q: slot %s fitted unknown weapon %q", h.HullID, slot.SlotID, weaponID)
			}
		}
	}
}

// Unknown ship types must fall back to a valid hull rather than returning a broken config.
func TestDefaultLoadout_UnknownFallsBackToFighter(t *testing.T) {
	cfg := DefaultLoadoutForShipType("does_not_exist")
	fighter := HullByStringID("fighter")
	if fighter == nil {
		t.Fatal("fighter hull missing from catalog")
	}
	if cfg.HullID != fighter.ID {
		t.Errorf("unknown type fallback HullID = %d, want fighter %d", cfg.HullID, fighter.ID)
	}
	if len(cfg.FittedWeapons) == 0 {
		t.Error("fallback loadout has no fitted weapons")
	}
}

// Miners should carry a mining laser rather than a combat weapon in their energy slot.
func TestDefaultLoadout_MinerGetsMiningLaser(t *testing.T) {
	cfg := DefaultLoadoutForShipType("miner")
	found := false
	for _, w := range cfg.FittedWeapons {
		if w == WeaponMiningLaserFit {
			found = true
		}
	}
	if !found {
		t.Errorf("miner loadout has no mining laser: %+v", cfg.FittedWeapons)
	}
}
