package domain

import (
	"context"
	"time"
)

type PlayerComponents struct {
	Transform  *Transform
	Velocity   *Velocity
	Health     *Health
	Shield     *Shield
	Weapon     *Weapon
	Cargo      *Cargo
	ShipConfig *ShipConfig
	Fleet      *Fleet
	Progress   *PlayerProgress
	Research   *PlayerResearch
}

type AsteroidSnapshot struct {
	EntityID  EntityID
	Transform Transform
	Resource  ResourceType
	Amount    int32
}

type StationSnapshot struct {
	EntityID     EntityID
	Transform    Transform
	FactionID    uint32
	Name         string
	Cargo        []ItemInstance
	Wallet       int64
	PlayerVaults map[uint64][]ItemInstance
	CorpVault    []ItemInstance
}

type JumpGateSnapshot struct {
	EntityID       EntityID
	Transform      Transform
	TargetSystemID uint32
	TargetX        float32
	TargetY        float32
}

type NPCSnapshot struct {
	EntityID  EntityID
	Name      string
	FactionID uint32
	CorpID    uint32
	Behavior  string
	Transform Transform
	Ships     []FleetShip
}

type WorldSnapshot struct {
	SystemID  uint32
	Asteroids []AsteroidSnapshot
	Stations  []StationSnapshot
	JumpGates []JumpGateSnapshot
	NPCs      []NPCSnapshot
}

// PlayerRepository represents the storage port for Player data.
type PlayerRepository interface {
	Save(ctx context.Context, player *PlayerData, components PlayerComponents) error
	Load(ctx context.Context, accountID uint64) (*PlayerData, PlayerComponents, error)
	ClearFleet(ctx context.Context, accountID uint64) error
}

// WorldRepository represents the storage port for static/persistent world entities.
type WorldRepository interface {
	SaveAsteroids(ctx context.Context, systemID uint32, asteroids []AsteroidSnapshot) error
	SaveStations(ctx context.Context, systemID uint32, stations []StationSnapshot) error
	LoadWorld(ctx context.Context, systemID uint32) (*WorldSnapshot, error)

	LoadPlayerVault(ctx context.Context, accountID uint64, stationID uint64) ([]ItemInstance, error)
	SavePlayerVault(ctx context.Context, accountID uint64, stationID uint64, items []ItemInstance) error
	LoadCorporationVault(ctx context.Context, corpID uint32, stationID uint64) ([]ItemInstance, error)
	SaveCorporationVault(ctx context.Context, corpID uint32, stationID uint64, items []ItemInstance) error
}

// SessionCache represents the fast-access temporary session storage.
type SessionCache interface {
	Set(ctx context.Context, sessionID string, accountID uint64, ttl time.Duration) error
	Get(ctx context.Context, sessionID string) (uint64, error)
	Delete(ctx context.Context, sessionID string) error
}

// EventBus represents the publish-subscribe messaging hub.
type EventBus interface {
	Publish(event Event)
	Subscribe(eventType string, handler func(Event))
}

type Corporation struct {
	ID        uint32
	Name      string
	Wallet    int64
	FounderID uint64
}

type CorporationRepository interface {
	Create(ctx context.Context, name string, founderID uint64) (*Corporation, error)
	Get(ctx context.Context, corpID uint32) (*Corporation, error)
	GetByName(ctx context.Context, name string) (*Corporation, error)
	AddMember(ctx context.Context, corpID uint32, accountID uint64, role string) error
	RemoveMember(ctx context.Context, accountID uint64) error
	GetMemberRole(ctx context.Context, accountID uint64) (uint32, string, error)  // returns corpID, role, err
	GetMembers(ctx context.Context, corpID uint32) (map[uint64]string, error)     // returns accountID -> role
	UpdateWallet(ctx context.Context, corpID uint32, amount int64) (int64, error) // returns new balance
}

type ShipRepository interface {
	SaveConfiguration(ctx context.Context, config *ShipConfiguration) error
	LoadConfiguration(ctx context.Context, configID uint64) (*ShipConfiguration, error)
	SaveFleet(ctx context.Context, fleet *CharacterFleet) error
	LoadFleet(ctx context.Context, ownerID uint64, ownerType string) (*CharacterFleet, error)
	ResolveHull(ctx context.Context, hullID uint32) (*ShipHull, error)
	ResolveWeapon(ctx context.Context, weaponID string) (*WeaponDefinition, error)
	ResolveHullmods(ctx context.Context, modIDs []string) (map[string]*Hullmod, error)
}
