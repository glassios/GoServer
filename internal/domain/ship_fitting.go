package domain

type WeaponSlot struct {
	SlotID string  `json:"slot_id"`
	Size   string  `json:"size"`   // SMALL, MEDIUM, LARGE
	Type   string  `json:"type"`   // BALLISTIC, ENERGY, MISSILE, UNIVERSAL, etc.
	Mount  string  `json:"mount"`  // TURRET, HARDPOINT, HIDDEN
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Angle  float32 `json:"angle"`
}

type ShipHull struct {
	ID               uint32       `json:"id"`
	HullID           string       `json:"hull_id"`
	Name             string       `json:"name"`
	BaseHP           float32      `json:"base_hp"`
	BaseArmor        float32      `json:"base_armor"`
	BaseShieldMax    float32      `json:"base_shield_max"`
	ShieldType       string       `json:"shield_type"` // front, omni, none
	ShieldArc        float32      `json:"shield_arc"`
	ShieldEfficiency float32      `json:"shield_efficiency"`
	BaseMaxSpeed     float32      `json:"base_max_speed"`
	BaseTurnRate     float32      `json:"base_turn_rate"`
	OrdnancePoints   int32        `json:"ordnance_points"`
	WeaponSlots      []WeaponSlot `json:"weapon_slots"`
}

type WeaponDefinition struct {
	ID            uint32  `json:"id"`
	WeaponID      string  `json:"weapon_id"`
	Name          string  `json:"name"`
	WeaponType    string  `json:"weapon_type"` // BALLISTIC, ENERGY, MISSILE
	WeaponSize    string  `json:"weapon_size"` // SMALL, MEDIUM, LARGE
	OPCost        int32   `json:"op_cost"`
	DamagePerShot float32 `json:"damage_per_shot"`
	DamageType    string  `json:"damage_type"` // KINETIC, EXPLOSIVE, ENERGY, FRAGMENTATION
	FluxCost      float32 `json:"flux_cost"`
	Range         float32 `json:"range"`
	Cooldown      float32 `json:"cooldown"`
}

type Hullmod struct {
	ID           uint32             `json:"id"`
	ModID        string             `json:"mod_id"`
	Name         string             `json:"name"`
	OPCostBySize map[string]int32   `json:"op_cost_by_size"` // FRIGATE, DESTROYER, etc.
	Modifiers    map[string]float32 `json:"modifiers"`        // e.g. "shield_damage_taken_mult": 0.8
}

type ShipConfiguration struct {
	ID             uint64            `json:"id"`
	OwnerID        uint64            `json:"owner_id"`
	OwnerType      string            `json:"owner_type"` // player, npc
	HullID         uint32            `json:"hull_id"`
	Hull           *ShipHull         `json:"hull,omitempty"` // populated if resolved
	CustomName     string            `json:"custom_name"`
	FittedWeapons  map[string]string `json:"fitted_weapons"`  // slot_id -> weapon_id
	FittedHullmods []string          `json:"fitted_hullmods"` // list of mod_id strings
	Vents          int32             `json:"vents"`
	Capacitors     int32             `json:"capacitors"`
}

type CharacterFleet struct {
	ID        uint64   `json:"id"`
	OwnerID   uint64   `json:"owner_id"`
	OwnerType string   `json:"owner_type"` // player, npc
	SystemID  uint32   `json:"system_id"`
	X         float32  `json:"x"`
	Y         float32  `json:"y"`
	ShipIDs   []uint64 `json:"ship_ids"`
}

// ECS battle components
type FluxState struct {
	Current         float32
	Capacity        float32
	DissipationRate float32
}

type FittedWeaponState struct {
	SlotID     string
	Definition WeaponDefinition
	Cooldown   float32
	Ammo       int32
}

type WeaponGroup struct {
	Weapons []FittedWeaponState
}
