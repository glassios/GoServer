package systems

import (
	"fmt"

	"github.com/Home/galaxy-mmo/internal/domain"
)

// fitting.go is the server-authoritative validation + preview layer for the Phase 2 hangar.
// All refit requests run through ValidateLoadout before being applied to a ship, so the client
// can never produce an illegal configuration (wrong weapon in a slot, over ordnance budget).

// weaponSizeRank lets us allow a smaller weapon in a larger slot (Starsector-style) but never
// the reverse.
var weaponSizeRank = map[string]int{"SMALL": 1, "MEDIUM": 2, "LARGE": 3}

// vent/capacitor ordnance cost (1 OP each, Starsector-style).
const ventCapOPCost = 1

// HullSizeClass maps a hull to the size bucket used for hullmod OP costs (OPCostBySize keys).
func HullSizeClass(hull *domain.ShipHull) string {
	switch {
	case hull.OrdnancePoints <= 70:
		return "FRIGATE"
	case hull.OrdnancePoints <= 130:
		return "DESTROYER"
	case hull.OrdnancePoints <= 200:
		return "CRUISER"
	default:
		return "CAPITAL"
	}
}

// resolveHull resolves the hull a configuration refers to (numeric id, else fighter fallback).
func resolveHull(cfg *domain.ShipConfiguration) *domain.ShipHull {
	if cfg.Hull != nil {
		return cfg.Hull
	}
	if h := domain.HullByNumericID(cfg.HullID); h != nil {
		return h
	}
	return domain.HullByStringID("fighter")
}

// ComputeOP returns the ordnance points used by a configuration and the hull's total budget.
func ComputeOP(cfg *domain.ShipConfiguration) (used int32, total int32) {
	hull := resolveHull(cfg)
	if hull == nil {
		return 0, 0
	}
	total = hull.OrdnancePoints
	sizeClass := HullSizeClass(hull)

	for _, wid := range cfg.FittedWeapons {
		if w := domain.WeaponByID(wid); w != nil {
			used += w.OPCost
		}
	}
	for _, modID := range cfg.FittedHullmods {
		if m := domain.HullmodByID(modID); m != nil {
			used += m.OPCostBySize[sizeClass]
		}
	}
	used += cfg.Vents * ventCapOPCost
	used += cfg.Capacitors * ventCapOPCost
	return used, total
}

// slotByID indexes a hull's weapon slots.
func slotByID(hull *domain.ShipHull, slotID string) *domain.WeaponSlot {
	for i := range hull.WeaponSlots {
		if hull.WeaponSlots[i].SlotID == slotID {
			return &hull.WeaponSlots[i]
		}
	}
	return nil
}

// weaponFitsSlot reports whether a weapon may be mounted in a slot (type + size compatibility).
func weaponFitsSlot(w *domain.WeaponDefinition, slot *domain.WeaponSlot) bool {
	if slot.Type != "UNIVERSAL" && w.WeaponType != slot.Type {
		return false
	}
	return weaponSizeRank[w.WeaponSize] <= weaponSizeRank[slot.Size]
}

// ValidateLoadout checks slot/weapon compatibility, known hullmods, non-negative vents/caps and
// the ordnance-point budget. Returns a descriptive error on the first violation.
func ValidateLoadout(cfg *domain.ShipConfiguration) error {
	hull := resolveHull(cfg)
	if hull == nil {
		return fmt.Errorf("unknown hull")
	}

	for slotID, wid := range cfg.FittedWeapons {
		if wid == "" {
			continue
		}
		slot := slotByID(hull, slotID)
		if slot == nil {
			return fmt.Errorf("hull %s has no slot %s", hull.HullID, slotID)
		}
		w := domain.WeaponByID(wid)
		if w == nil {
			return fmt.Errorf("unknown weapon %s", wid)
		}
		if !weaponFitsSlot(w, slot) {
			return fmt.Errorf("weapon %s (%s %s) does not fit slot %s (%s %s)",
				w.WeaponID, w.WeaponSize, w.WeaponType, slot.SlotID, slot.Size, slot.Type)
		}
	}

	for _, modID := range cfg.FittedHullmods {
		if domain.HullmodByID(modID) == nil {
			return fmt.Errorf("unknown hullmod %s", modID)
		}
	}

	if cfg.Vents < 0 || cfg.Capacitors < 0 {
		return fmt.Errorf("vents/capacitors cannot be negative")
	}

	used, total := ComputeOP(cfg)
	if used > total {
		return fmt.Errorf("over ordnance budget: %d/%d OP", used, total)
	}
	return nil
}
