package domain

import "strings"

// fitting_catalog.go is the CANONICAL, code-defined catalog of ship hulls, weapons and
// hullmods plus the "stock" loadouts used as defaults for players and NPCs.
//
// It is the single source of truth for the Starsector-style fitting system:
//   - The in-memory ShipRepository (DB-less mode) serves directly from this catalog.
//   - Migration 009_fitting_seed.sql mirrors the SAME data into Postgres, so the
//     DB-backed ShipRepository resolves identically (numeric hull IDs and the
//     weapon_id / mod_id strings here MUST match that migration).
//
// Phase 0 keeps stats on the existing combat scale (hull HP ~70-800, weapon damage
// ~8-100) so activating BakeShip in Phase 1 does not jar current balance.

// Stock weapon IDs (must match weapon_definitions.weapon_id in migration 009).
const (
	WeaponLightLaser      = "light_laser"
	WeaponIRPulse         = "ir_pulse_laser"
	WeaponLightAutocannon = "light_autocannon"
	WeaponLightMortar     = "light_mortar"
	WeaponSwarmerSRM      = "swarmer_srm"
	WeaponHeavyBlaster    = "heavy_blaster"
	WeaponHeavyMauler     = "heavy_mauler"
	WeaponHellbore        = "hellbore"
	WeaponMiningLaserFit  = "mining_laser" // weak utility energy weapon for miners
)

// Stock hullmod IDs (must match hullmods.mod_id in migration 009).
const (
	HullmodReinforcedBulkheads = "reinforced_bulkheads"
	HullmodAugmentedEngines    = "augmented_engines"
	HullmodHardenedShields     = "hardened_shields"
	HullmodFluxCoilAdjunct     = "flux_coil_adjunct"
)

// StockHulls is the canonical hull catalog. The numeric ID is stable and is what
// ShipConfiguration.HullID / ShipRepository.ResolveHull(id) refer to. HullID (string)
// doubles as the ship-type key, so the existing FleetShip.ShipType maps directly onto a hull.
var StockHulls = []ShipHull{
	{
		ID: 1, HullID: "fighter", Name: "Fighter",
		BaseHP: 100, BaseArmor: 40, BaseShieldMax: 80,
		ShieldType: "front", ShieldArc: 120, ShieldEfficiency: 1.0,
		BaseMaxSpeed: 120, BaseTurnRate: 1.8, OrdnancePoints: 60,
		WeaponSlots: []WeaponSlot{
			{SlotID: "WS1", Size: "SMALL", Type: "ENERGY", Mount: "HARDPOINT", X: 12, Y: 0, Angle: 0},
			{SlotID: "WS2", Size: "SMALL", Type: "BALLISTIC", Mount: "TURRET", X: -4, Y: 6, Angle: 0},
		},
	},
	{
		ID: 2, HullID: "patrol", Name: "Patrol Cutter",
		BaseHP: 140, BaseArmor: 70, BaseShieldMax: 120,
		ShieldType: "omni", ShieldArc: 180, ShieldEfficiency: 1.0,
		BaseMaxSpeed: 100, BaseTurnRate: 1.5, OrdnancePoints: 90,
		WeaponSlots: []WeaponSlot{
			{SlotID: "WS1", Size: "MEDIUM", Type: "ENERGY", Mount: "TURRET", X: 8, Y: 0, Angle: 0},
			{SlotID: "WS2", Size: "SMALL", Type: "BALLISTIC", Mount: "TURRET", X: -6, Y: 5, Angle: 0},
			{SlotID: "WS3", Size: "SMALL", Type: "BALLISTIC", Mount: "TURRET", X: -6, Y: -5, Angle: 0},
		},
	},
	{
		ID: 3, HullID: "pirate", Name: "Pirate Raider",
		BaseHP: 90, BaseArmor: 40, BaseShieldMax: 60,
		ShieldType: "front", ShieldArc: 90, ShieldEfficiency: 0.9,
		BaseMaxSpeed: 90, BaseTurnRate: 1.4, OrdnancePoints: 55,
		WeaponSlots: []WeaponSlot{
			{SlotID: "WS1", Size: "SMALL", Type: "BALLISTIC", Mount: "HARDPOINT", X: 12, Y: 0, Angle: 0},
			{SlotID: "WS2", Size: "SMALL", Type: "MISSILE", Mount: "HARDPOINT", X: 8, Y: 6, Angle: 0},
		},
	},
	{
		ID: 4, HullID: "miner", Name: "Mining Barge",
		BaseHP: 120, BaseArmor: 60, BaseShieldMax: 50,
		ShieldType: "front", ShieldArc: 120, ShieldEfficiency: 1.0,
		BaseMaxSpeed: 60, BaseTurnRate: 1.0, OrdnancePoints: 40,
		WeaponSlots: []WeaponSlot{
			{SlotID: "WS1", Size: "SMALL", Type: "ENERGY", Mount: "TURRET", X: 6, Y: 0, Angle: 0},
		},
	},
	{
		ID: 5, HullID: "cargo_helper", Name: "Cargo Hauler",
		BaseHP: 140, BaseArmor: 50, BaseShieldMax: 40,
		ShieldType: "front", ShieldArc: 90, ShieldEfficiency: 1.0,
		BaseMaxSpeed: 50, BaseTurnRate: 0.8, OrdnancePoints: 30,
		WeaponSlots: []WeaponSlot{
			{SlotID: "WS1", Size: "SMALL", Type: "BALLISTIC", Mount: "TURRET", X: 4, Y: 0, Angle: 0},
		},
	},
	{
		ID: 6, HullID: "interceptor", Name: "Interceptor",
		BaseHP: 70, BaseArmor: 20, BaseShieldMax: 50,
		ShieldType: "front", ShieldArc: 90, ShieldEfficiency: 1.0,
		BaseMaxSpeed: 140, BaseTurnRate: 2.2, OrdnancePoints: 45,
		WeaponSlots: []WeaponSlot{
			{SlotID: "WS1", Size: "SMALL", Type: "ENERGY", Mount: "HARDPOINT", X: 10, Y: 0, Angle: 0},
		},
	},
	{
		ID: 7, HullID: "destroyer", Name: "Destroyer",
		BaseHP: 400, BaseArmor: 200, BaseShieldMax: 400,
		ShieldType: "omni", ShieldArc: 180, ShieldEfficiency: 1.0,
		BaseMaxSpeed: 70, BaseTurnRate: 0.9, OrdnancePoints: 130,
		WeaponSlots: []WeaponSlot{
			{SlotID: "WS1", Size: "MEDIUM", Type: "BALLISTIC", Mount: "TURRET", X: 14, Y: 0, Angle: 0},
			{SlotID: "WS2", Size: "MEDIUM", Type: "ENERGY", Mount: "TURRET", X: -8, Y: 8, Angle: 0},
			{SlotID: "WS3", Size: "SMALL", Type: "MISSILE", Mount: "HARDPOINT", X: 10, Y: -8, Angle: 0},
		},
	},
	{
		ID: 8, HullID: "cruiser", Name: "Cruiser",
		BaseHP: 800, BaseArmor: 400, BaseShieldMax: 700,
		ShieldType: "omni", ShieldArc: 220, ShieldEfficiency: 1.0,
		BaseMaxSpeed: 55, BaseTurnRate: 0.6, OrdnancePoints: 200,
		WeaponSlots: []WeaponSlot{
			{SlotID: "WS1", Size: "LARGE", Type: "BALLISTIC", Mount: "HARDPOINT", X: 18, Y: 0, Angle: 0},
			{SlotID: "WS2", Size: "MEDIUM", Type: "ENERGY", Mount: "TURRET", X: -6, Y: 10, Angle: 0},
			{SlotID: "WS3", Size: "MEDIUM", Type: "ENERGY", Mount: "TURRET", X: -6, Y: -10, Angle: 0},
			{SlotID: "WS4", Size: "SMALL", Type: "MISSILE", Mount: "HARDPOINT", X: 12, Y: 12, Angle: 0},
		},
	},
}

// StockWeapons is the canonical weapon catalog (weapon_id is the resolve key).
var StockWeapons = []WeaponDefinition{
	{ID: 1, WeaponID: WeaponLightLaser, Name: "Light Laser", WeaponType: "ENERGY", WeaponSize: "SMALL", OPCost: 5, DamagePerShot: 8, DamageType: "ENERGY", FluxCost: 6, Range: 450, Cooldown: 0.4},
	{ID: 2, WeaponID: WeaponIRPulse, Name: "IR Pulse Laser", WeaponType: "ENERGY", WeaponSize: "SMALL", OPCost: 6, DamagePerShot: 12, DamageType: "ENERGY", FluxCost: 10, Range: 400, Cooldown: 0.6},
	{ID: 3, WeaponID: WeaponLightAutocannon, Name: "Light Autocannon", WeaponType: "BALLISTIC", WeaponSize: "SMALL", OPCost: 6, DamagePerShot: 10, DamageType: "KINETIC", FluxCost: 8, Range: 500, Cooldown: 0.5},
	{ID: 4, WeaponID: WeaponLightMortar, Name: "Light Mortar", WeaponType: "BALLISTIC", WeaponSize: "SMALL", OPCost: 5, DamagePerShot: 14, DamageType: "EXPLOSIVE", FluxCost: 9, Range: 350, Cooldown: 0.8},
	{ID: 5, WeaponID: WeaponSwarmerSRM, Name: "Swarmer SRM", WeaponType: "MISSILE", WeaponSize: "SMALL", OPCost: 4, DamagePerShot: 20, DamageType: "EXPLOSIVE", FluxCost: 0, Range: 600, Cooldown: 2.0},
	{ID: 6, WeaponID: WeaponHeavyBlaster, Name: "Heavy Blaster", WeaponType: "ENERGY", WeaponSize: "MEDIUM", OPCost: 12, DamagePerShot: 45, DamageType: "ENERGY", FluxCost: 45, Range: 400, Cooldown: 1.0},
	{ID: 7, WeaponID: WeaponHeavyMauler, Name: "Heavy Mauler", WeaponType: "BALLISTIC", WeaponSize: "MEDIUM", OPCost: 12, DamagePerShot: 40, DamageType: "EXPLOSIVE", FluxCost: 30, Range: 700, Cooldown: 1.2},
	{ID: 8, WeaponID: WeaponHellbore, Name: "Hellbore Cannon", WeaponType: "BALLISTIC", WeaponSize: "LARGE", OPCost: 22, DamagePerShot: 100, DamageType: "EXPLOSIVE", FluxCost: 80, Range: 900, Cooldown: 2.5},
	{ID: 9, WeaponID: WeaponMiningLaserFit, Name: "Mining Laser", WeaponType: "ENERGY", WeaponSize: "SMALL", OPCost: 3, DamagePerShot: 5, DamageType: "ENERGY", FluxCost: 4, Range: 300, Cooldown: 1.0},
}

// StockHullmods is the canonical hullmod catalog. Only modifier keys that BakeShip reads
// are meaningful: max_speed_mult, turn_rate_mult, armor_mult, shield_max_mult,
// shield_efficiency_mult, max_flux_mult, flux_dissipation_mult.
var StockHullmods = []Hullmod{
	{ID: 1, ModID: HullmodReinforcedBulkheads, Name: "Reinforced Bulkheads", OPCostBySize: defaultOPBySize(), Modifiers: map[string]float32{"armor_mult": 1.25}},
	{ID: 2, ModID: HullmodAugmentedEngines, Name: "Augmented Engines", OPCostBySize: defaultOPBySize(), Modifiers: map[string]float32{"max_speed_mult": 1.4, "turn_rate_mult": 1.25}},
	{ID: 3, ModID: HullmodHardenedShields, Name: "Hardened Shields", OPCostBySize: defaultOPBySize(), Modifiers: map[string]float32{"shield_max_mult": 1.2, "shield_efficiency_mult": 0.85}},
	{ID: 4, ModID: HullmodFluxCoilAdjunct, Name: "Flux Coil Adjunct", OPCostBySize: defaultOPBySize(), Modifiers: map[string]float32{"flux_dissipation_mult": 1.3, "max_flux_mult": 1.1}},
}

func defaultOPBySize() map[string]int32 {
	return map[string]int32{"FRIGATE": 5, "DESTROYER": 10, "CRUISER": 15, "CAPITAL": 25}
}

// Lookup indexes built once at init.
var (
	hullByNumericID = map[uint32]*ShipHull{}
	hullByStringID  = map[string]*ShipHull{}
	weaponByID      = map[string]*WeaponDefinition{}
	hullmodByID     = map[string]*Hullmod{}
)

func init() {
	for i := range StockHulls {
		h := &StockHulls[i]
		hullByNumericID[h.ID] = h
		hullByStringID[h.HullID] = h
	}
	for i := range StockWeapons {
		w := &StockWeapons[i]
		weaponByID[w.WeaponID] = w
	}
	for i := range StockHullmods {
		m := &StockHullmods[i]
		hullmodByID[m.ModID] = m
	}
}

// HullByNumericID returns a copy of the catalog hull with the given numeric ID, or nil.
func HullByNumericID(id uint32) *ShipHull {
	if h, ok := hullByNumericID[id]; ok {
		c := *h
		return &c
	}
	return nil
}

// HullByStringID returns a copy of the catalog hull with the given string hull_id, or nil.
func HullByStringID(hullID string) *ShipHull {
	if h, ok := hullByStringID[strings.ToLower(hullID)]; ok {
		c := *h
		return &c
	}
	return nil
}

// WeaponByID returns a copy of the catalog weapon, or nil.
func WeaponByID(weaponID string) *WeaponDefinition {
	if w, ok := weaponByID[weaponID]; ok {
		c := *w
		return &c
	}
	return nil
}

// HullmodByID returns a copy of the catalog hullmod, or nil.
func HullmodByID(modID string) *Hullmod {
	if m, ok := hullmodByID[modID]; ok {
		c := *m
		return &c
	}
	return nil
}

// BakedStats are the flat, combat-ready stats derived from a ShipConfiguration (hull + mods +
// weapons + vents/capacitors). Computed purely from the in-code catalog — no repository needed —
// so the combat instance can bake ships in both DB and DB-less modes. Mirrors the math in
// systems.BakeShip (the repository-backed path used by the hangar/DB).
type BakedStats struct {
	MaxHP            float32
	MaxArmor         float32
	MaxShield        float32
	ShieldType       string
	ShieldArc        float32
	ShieldEfficiency float32
	MaxSpeed         float32
	TurnRate         float32
	MaxFlux          float32
	FluxDissipation  float32
	Weapons          []FittedWeaponState
}

// ComputeStats resolves a ShipConfiguration into BakedStats using the code catalog.
func ComputeStats(cfg *ShipConfiguration) BakedStats {
	hull := cfg.Hull
	if hull == nil {
		hull = HullByNumericID(cfg.HullID)
	}
	if hull == nil {
		hull = HullByStringID("fighter")
	}

	speedMult, turnMult, armorMult := float32(1), float32(1), float32(1)
	shieldCapMult, shieldEffMult := float32(1), float32(1)
	maxFluxMult, fluxDissMult := float32(1), float32(1)
	for _, modID := range cfg.FittedHullmods {
		m := HullmodByID(modID)
		if m == nil {
			continue
		}
		if v, ok := m.Modifiers["max_speed_mult"]; ok {
			speedMult *= v
		}
		if v, ok := m.Modifiers["turn_rate_mult"]; ok {
			turnMult *= v
		}
		if v, ok := m.Modifiers["armor_mult"]; ok {
			armorMult *= v
		}
		if v, ok := m.Modifiers["shield_max_mult"]; ok {
			shieldCapMult *= v
		}
		if v, ok := m.Modifiers["shield_efficiency_mult"]; ok {
			shieldEffMult *= v
		}
		if v, ok := m.Modifiers["max_flux_mult"]; ok {
			maxFluxMult *= v
		}
		if v, ok := m.Modifiers["flux_dissipation_mult"]; ok {
			fluxDissMult *= v
		}
	}

	eff := hull.ShieldEfficiency * shieldEffMult
	if eff <= 0 {
		eff = 1.0
	}

	stats := BakedStats{
		MaxHP:            hull.BaseHP,
		MaxArmor:         hull.BaseArmor * armorMult,
		MaxShield:        hull.BaseShieldMax * shieldCapMult,
		ShieldType:       hull.ShieldType,
		ShieldArc:        hull.ShieldArc,
		ShieldEfficiency: eff,
		MaxSpeed:         hull.BaseMaxSpeed * speedMult,
		TurnRate:         hull.BaseTurnRate * turnMult,
		MaxFlux:          (hull.BaseHP*10 + float32(cfg.Capacitors)*200.0) * maxFluxMult,
		FluxDissipation:  (float32(hull.OrdnancePoints)/2.0 + float32(cfg.Vents)*10.0) * fluxDissMult,
	}
	for _, slot := range hull.WeaponSlots {
		if wid, ok := cfg.FittedWeapons[slot.SlotID]; ok {
			if def := WeaponByID(wid); def != nil {
				stats.Weapons = append(stats.Weapons, FittedWeaponState{SlotID: slot.SlotID, Definition: *def, Cooldown: 0, Ammo: 9999})
			}
		}
	}
	return stats
}

// defaultWeaponForSlot picks a sensible stock weapon for a slot's size + mount type.
func defaultWeaponForSlot(slot WeaponSlot) string {
	switch slot.Size {
	case "LARGE":
		return WeaponHellbore
	case "MEDIUM":
		if slot.Type == "ENERGY" {
			return WeaponHeavyBlaster
		}
		return WeaponHeavyMauler
	default: // SMALL
		switch slot.Type {
		case "MISSILE":
			return WeaponSwarmerSRM
		case "BALLISTIC":
			return WeaponLightAutocannon
		default: // ENERGY / UNIVERSAL
			return WeaponLightLaser
		}
	}
}

// DefaultLoadoutForShipType returns a ready-to-bake stock ShipConfiguration for the given
// ship-type / hull string. Unknown types fall back to the "fighter" hull so callers always
// receive a valid, fittable configuration. The returned config leaves Hull nil so BakeShip
// resolves it through the repository (works for both in-memory and DB-backed repos).
func DefaultLoadoutForShipType(shipType string) *ShipConfiguration {
	hull := hullByStringID[strings.ToLower(shipType)]
	if hull == nil {
		hull = hullByStringID["fighter"]
	}

	fitted := make(map[string]string, len(hull.WeaponSlots))
	for _, slot := range hull.WeaponSlots {
		// Miners get a mining laser in their (single) energy slot instead of a real gun.
		if hull.HullID == "miner" && slot.Type == "ENERGY" {
			fitted[slot.SlotID] = WeaponMiningLaserFit
			continue
		}
		fitted[slot.SlotID] = defaultWeaponForSlot(slot)
	}

	return &ShipConfiguration{
		HullID:         hull.ID,
		OwnerType:      "npc",
		CustomName:     hull.Name,
		FittedWeapons:  fitted,
		FittedHullmods: []string{},
		Vents:          5,
		Capacitors:     5,
	}
}
