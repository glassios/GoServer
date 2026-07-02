package domain

type WeaponSlot struct {
	SlotID string  `json:"slot_id"`
	Size   string  `json:"size"`  // SMALL, MEDIUM, LARGE
	Type   string  `json:"type"`  // BALLISTIC, ENERGY, MISSILE, UNIVERSAL, etc.
	Mount  string  `json:"mount"` // TURRET, HARDPOINT, HIDDEN
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
	// ModuleItem (Phase 4) is the cargo item this weapon is crafted as. When non-empty the weapon
	// is a "module": fitting it consumes one item from the player's cargo and unfitting returns it.
	// Empty means a built-in/basic weapon that is always available.
	ModuleItem string `json:"module_item"`
	// Class (Phase B3) selects how the shot presents. Empty / "hitscan" = instant beam-line
	// (default, unchanged balance). "projectile" = the shot travels: damage is still applied
	// instantly on the server, but a fire event is streamed so the client draws the bolt flying
	// (thin channel, IMPLEMENTATION_PLAN §2.2). ProjectileSpeed is world units/sec for that flight.
	Class           string  `json:"class"`
	ProjectileSpeed float32 `json:"projectile_speed"`
	// Magazine + reload (Phase B5, turret rhythm): a weapon with Magazine>0 fires that many shots
	// (at its Cooldown cadence) then must reload for ReloadTime seconds before it can fire again.
	// Magazine==0 means no magazine (fires continuously, gated only by Cooldown) — the default.
	Magazine   int32   `json:"magazine"`
	ReloadTime float32 `json:"reload_time"`
	// Barrels (Phase B5): shots discharged per trigger (a volley). 0/1 = single shot. A magazine
	// round covers one trigger, so a Barrels-N mount fires N shots per round consumed.
	Barrels int32 `json:"barrels"`
	// Guidance (Phase B4, missile class only): "" = straight homing, "weave" = sinusoidal weave
	// toward the target (harder for point-defense, swarm feel).
	Guidance string `json:"guidance"`
}

// Weapon classes (Phase B3).
const (
	WeaponClassHitscan    = "hitscan"
	WeaponClassProjectile = "projectile"
	WeaponClassBeam       = "beam"    // continuous/pulse energy beam (instant-travel line, not a bolt)
	WeaponClassMissile    = "missile" // guided munition: travels + homes (client-side cosmetic homing)
)

type Hullmod struct {
	ID           uint32             `json:"id"`
	ModID        string             `json:"mod_id"`
	Name         string             `json:"name"`
	OPCostBySize map[string]int32   `json:"op_cost_by_size"` // FRIGATE, DESTROYER, etc.
	Modifiers    map[string]float32 `json:"modifiers"`       // e.g. "shield_damage_taken_mult": 0.8
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
	Overloaded      bool    // true while at max flux: shield drops and weapons can't fire
	Venting         bool    // true while actively venting (fast flux dump, can't fire)
	OverloadTimer   float32 // seconds remaining in a fixed-duration overload (Starsector-style); overload ends when it hits 0, flux resets to 0
}

// Damage types (Starsector-style).
const (
	DamageKinetic   = "KINETIC"
	DamageExplosive = "EXPLOSIVE"
	DamageEnergy    = "ENERGY"
	DamageFrag      = "FRAGMENTATION"
)

// Damage layers.
const (
	LayerShield = "shield"
	LayerArmor  = "armor"
	LayerHull   = "hull"
)

// DamageMultiplier returns the Starsector-style multiplier for a damage type against a
// given defensive layer. Kinetic shreds shields but is weak vs armor; explosive is the
// inverse; energy is neutral; fragmentation is strong vs hull but poor vs shields/armor.
func DamageMultiplier(damageType, layer string) float32 {
	switch damageType {
	case DamageKinetic:
		switch layer {
		case LayerShield:
			return 2.0
		case LayerArmor:
			return 0.5
		default:
			return 1.0
		}
	case DamageExplosive:
		switch layer {
		case LayerShield:
			return 0.5
		case LayerArmor:
			return 2.0
		default:
			return 1.0
		}
	case DamageFrag:
		switch layer {
		case LayerShield:
			return 0.25
		case LayerArmor:
			return 0.25
		default:
			return 1.0
		}
	default: // ENERGY and anything unspecified: neutral
		return 1.0
	}
}

type FittedWeaponState struct {
	SlotID      string
	Definition  WeaponDefinition
	Cooldown    float32
	Ammo        int32   // shots left in the current magazine (Phase B5)
	ReloadTimer float32 // seconds left until the magazine refills (0 = ready/not reloading)
	// Per-mount firing arc (Phase B5): the mount can only fire when the target lies within ArcHalf
	// radians of (hull facing + MountAngle). Turrets get a wide arc, hardpoints a narrow forward
	// one, so ship facing matters. ArcHalf >= π means "no gate" (fires in any direction).
	MountAngle float32
	ArcHalf    float32
}

const degToRad = 0.017453292519943295

// MountArcHalf returns the half firing arc (radians) for a mount type: hardpoints are fixed and
// fire in a narrow forward cone; turrets traverse over a wide arc; anything else is unrestricted.
func MountArcHalf(mount string) float32 {
	switch mount {
	case "HARDPOINT":
		return 45 * degToRad // 90° cone
	case "TURRET":
		return 135 * degToRad // 270° coverage
	default:
		return 180 * degToRad // full (no gate)
	}
}

// InitialAmmo is the round count a freshly-baked mount starts with: a full magazine for magazine
// weapons, or an effectively unlimited count for continuous ones.
func (d *WeaponDefinition) InitialAmmo() int32 {
	if d.Magazine > 0 {
		return d.Magazine
	}
	return 9999
}

type WeaponGroup struct {
	Weapons []FittedWeaponState
}
