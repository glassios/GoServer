package domain

type ResourceType string

const (
	ResourceIron          ResourceType = "Iron"
	ResourceTitanium      ResourceType = "Titanium"
	ResourceCrystal       ResourceType = "Crystal"
	ResourceRareGas       ResourceType = "RareGas"
	ResourceSiliconWafers ResourceType = "SiliconWafers"
	ResourceFuelCells     ResourceType = "FuelCells"
	ResourceMicrochips    ResourceType = "Microchips"
	ResourceEnergyCoils   ResourceType = "EnergyCoils"
)

type WeaponType string

const (
	WeaponLaser   WeaponType = "Laser"
	WeaponPlasma  WeaponType = "Plasma"
	WeaponRailgun WeaponType = "Railgun"
)

type AIBehavior string

const (
	BehaviorIdle    AIBehavior = "Idle"
	BehaviorPatrol  AIBehavior = "Patrol"
	BehaviorAttack  AIBehavior = "Attack"
	BehaviorMine    AIBehavior = "Mine"
	BehaviorRetreat AIBehavior = "Retreat"
	BehaviorDock    AIBehavior = "Dock"
	BehaviorEscort  AIBehavior = "Escort"
	BehaviorDefend  AIBehavior = "Defend"
)

type Transform struct {
	X        float32
	Y        float32
	Rotation float32 // в радианах или градусах
}

type Velocity struct {
	X float32
	Y float32
}

type Health struct {
	Current int32
	Max     int32
}

type Shield struct {
	Current   int32
	Max       int32
	RegenRate float32 // points restored per second while raised

	// Starsector-style fields (Phase 1). Defaults are backwards-compatible:
	// Down=false means the shield is raised; an empty Type behaves like "omni".
	Type       string  // "front", "omni", "none"
	Arc        float32 // covered arc in degrees (used for directional shields)
	Efficiency float32 // flux generated per point of damage absorbed (lower = better); 0 → treated as 1.0
	Down       bool    // true when the shield is dropped (e.g. on flux overload)
	RegenAcc   float32 // fractional regen accumulator (Current is int32; carries sub-point regen between ticks)
}

// ArmorGrid is the armor layer that sits between shields and hull (Phase 1).
// Armor does not regenerate in combat (the repair role restores it in Phase 1.5).
type ArmorGrid struct {
	Current float32
	Max     float32
}

// CombatFx carries transient, per-tick combat outputs for the snapshot/visualizer:
// how many weapon mounts fired this tick and the damage type of the most recent hit taken.
type CombatFx struct {
	ShotsFired     int32
	LastDamageType string
}

type Weapon struct {
	Type     WeaponType
	Damage   int32
	Range    float32
	Cooldown float32
	LastFire float64 // time stamp
	Active   bool
	TargetID EntityID
	IsFiring bool // Temporary state to indicate actual weapon discharge in the current tick
}

type Cargo struct {
	Items    []ItemInstance
	Capacity int32
}

type MiningLaser struct {
	Power    float32
	Range    float32
	Active   bool
	TargetID EntityID
	LastMine float64 // time stamp of last extraction tick
}

type AsteroidResource struct {
	Type   ResourceType
	Amount int32
}

type AIState struct {
	Behavior   AIBehavior
	TargetID   EntityID
	HomePos    Transform
	StateTimer float64
}

type FactionMember struct {
	FactionID  uint32
	Reputation map[uint32]int32 // faction_id -> reputation value
}

type ShipConfig struct {
	ShipType string
	MaxSpeed float32
	TurnRate float32
}

type PlayerData struct {
	AccountID uint64
	Name      string
	Credits   int64
	SessionID string
	SystemID  uint32 // Added to persist the player's current system ID
}

type Visibility struct {
	Radius          float32
	VisibleEntities map[EntityID]struct{}
}

type MarketItem struct {
	BasePrice int32
	Supply    int32
	Demand    int32
}

type StationMarket struct {
	Items map[ResourceType]*MarketItem
}

type SystemMember struct {
	SystemID uint32
}

type JumpGate struct {
	TargetSystemID uint32
	TargetX        float32
	TargetY        float32
}

type FleetShip struct {
	ShipID        uint32
	ShipType      string
	Health        int32
	MaxHealth     int32
	Shield        int32
	MaxShield     int32
	CargoCapacity int32

	// Phase 1.5 per-ship tactics, set by the player before battle (independent of hull type).
	// Empty values fall back to sensible defaults at unpack time.
	Role     string // "tank", "dps", "support", "repair"
	Strategy string // "attack", "defense", "retreat"

	// Phase 2 per-ship fitting (loadout). When Customized is false the ship uses the stock
	// loadout for its hull type (DefaultLoadoutForShipType); when true the fields below define
	// the player's refit. See EffectiveConfig.
	Customized     bool
	HullID         uint32            // numeric catalog hull id; 0 => derive from ShipType
	FittedWeapons  map[string]string // slot_id -> weapon_id
	FittedHullmods []string          // mod_id list
	Vents          int32
	Capacitors     int32
}

// EffectiveConfig resolves a ship's fitting into a ShipConfiguration ready for ComputeStats /
// BakeShip. Uncustomized ships fall back to the stock loadout for their hull type, so battles and
// the hangar always have a valid, fully-fitted configuration to work from.
func (fs *FleetShip) EffectiveConfig() *ShipConfiguration {
	if !fs.Customized {
		return DefaultLoadoutForShipType(fs.ShipType)
	}
	hullID := fs.HullID
	if hullID == 0 {
		if h := HullByStringID(fs.ShipType); h != nil {
			hullID = h.ID
		}
	}
	weapons := make(map[string]string, len(fs.FittedWeapons))
	for slot, wid := range fs.FittedWeapons {
		if wid != "" {
			weapons[slot] = wid
		}
	}
	return &ShipConfiguration{
		HullID:         hullID,
		OwnerType:      "player",
		CustomName:     fs.ShipType,
		FittedWeapons:  weapons,
		FittedHullmods: append([]string{}, fs.FittedHullmods...),
		Vents:          fs.Vents,
		Capacitors:     fs.Capacitors,
	}
}

type Fleet struct {
	Ships []FleetShip
}

type CorporationMember struct {
	CorpID uint32
	Role   string
}

type StationOwnership struct {
	CorpID uint32
}

type Refinery struct {
	IsActive bool
	Progress float32
	Yield    float32
}

type ShipBuildOrder struct {
	ShipType  string
	Progress  float32
	TotalTime float32
	OrderedBy uint64
}

type Shipyard struct {
	Queue    []ShipBuildOrder
	Progress float32
}

type Loot struct {
	Credits int64
}

type StationVaults struct {
	PlayerVaults map[uint64][]ItemInstance
}

type CorporationVault struct {
	OwnerCorpID uint32
	Items       []ItemInstance
}

type ItemDefinition struct {
	ID        uint32                 `json:"id"`
	Name      string                 `json:"name"`
	Category  string                 `json:"category"`
	Stackable bool                   `json:"stackable"`
	Volume    float32                `json:"volume"`
	MetaData  map[string]interface{} `json:"meta_data"`
}

type ItemInstance struct {
	ID           uint64                 `json:"id"`
	DefinitionID uint32                 `json:"definition_id"`
	Quantity     int32                  `json:"quantity"`
	LocationType string                 `json:"location_type"`
	LocationID   uint64                 `json:"location_id"`
	OwnerID      uint64                 `json:"owner_id,omitempty"`
	State        string                 `json:"state"`
	MetaData     map[string]interface{} `json:"meta_data,omitempty"`
}

var ResourceToID = map[ResourceType]uint32{
	ResourceIron:          1,
	ResourceTitanium:      2,
	ResourceCrystal:       3,
	ResourceRareGas:       4,
	"IronPlates":          5,
	"TitaniumPlates":      6,
	"Laser Cannon":        7,
	"Mining Laser":        8,
	ResourceSiliconWafers: 11,
	ResourceFuelCells:     12,
	ResourceMicrochips:    13,
	ResourceEnergyCoils:   14,
}

var IDToResource = map[uint32]ResourceType{
	1:  ResourceIron,
	2:  ResourceTitanium,
	3:  ResourceCrystal,
	4:  ResourceRareGas,
	5:  "IronPlates",
	6:  "TitaniumPlates",
	7:  "Laser Cannon",
	8:  "Mining Laser",
	11: ResourceSiliconWafers,
	12: ResourceFuelCells,
	13: ResourceMicrochips,
	14: ResourceEnergyCoils,
}

func (c *Cargo) GetQuantity(defID uint32) int32 {
	var total int32
	for _, item := range c.Items {
		if item.DefinitionID == defID {
			total += item.Quantity
		}
	}
	return total
}

func (c *Cargo) AddItem(defID uint32, qty int32) {
	// All seed resources/materials are stackable
	isStackable := defID <= 6
	if isStackable {
		for i, item := range c.Items {
			if item.DefinitionID == defID {
				c.Items[i].Quantity += qty
				return
			}
		}
	}
	c.Items = append(c.Items, ItemInstance{
		DefinitionID: defID,
		Quantity:     qty,
		State:        "normal",
	})
}

func (c *Cargo) GetResourceTypeQuantity(res ResourceType) int32 {
	id, exists := ResourceToID[res]
	if !exists {
		return 0
	}
	return c.GetQuantity(id)
}

func (c *Cargo) AddResourceTypeQuantity(res ResourceType, qty int32) {
	id, exists := ResourceToID[res]
	if !exists {
		return
	}
	c.AddItem(id, qty)
}

func (c *Cargo) RemoveResourceTypeQuantity(res ResourceType, qty int32) bool {
	id, exists := ResourceToID[res]
	if !exists {
		return false
	}
	for i, item := range c.Items {
		if item.DefinitionID == id {
			if item.Quantity >= qty {
				c.Items[i].Quantity -= qty
				if c.Items[i].Quantity <= 0 {
					c.Items = append(c.Items[:i], c.Items[i+1:]...)
				}
				return true
			}
			return false
		}
	}
	return false
}

type CombatState struct {
	InCombat         bool
	CombatInstanceID uint32
	OpponentID       EntityID
}

type CombatTeam struct {
	TeamID  uint32
	FleetID EntityID
}

// Combat roles & strategies (Phase 1.5). Assigned per ship before battle; drive the
// tactical AI. Roles are RPG-style and independent of hull type.
const (
	RoleTank    = "tank"
	RoleDPS     = "dps"
	RoleSupport = "support"
	RoleRepair  = "repair"

	StanceAttack  = "attack"
	StanceDefense = "defense"
	StanceRetreat = "retreat"
)

// CombatRole drives role-specific behavior (positioning, target priority, abilities).
type CombatRole struct {
	Role           string
	AssistTargetID EntityID // ally currently being repaired/supported (for FX/snapshot)
	AbilityTimer   float64  // generic per-role ability accumulator
}

// CombatStrategy is the ship's stance for the engagement.
type CombatStrategy struct {
	Stance string
}

type CombatMarker struct {
	CombatInstanceID uint32
	AttackerID       EntityID
	DefenderID       EntityID
}

type MinerAttacker struct {
	IsCriminal bool
}
