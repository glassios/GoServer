package domain

import "time"

type Event interface {
	EventType() string
	Timestamp() time.Time
}

type BaseEvent struct {
	Time time.Time
}

func (b BaseEvent) Timestamp() time.Time {
	return b.Time
}

type EntityDestroyedEvent struct {
	BaseEvent
	EntityID   EntityID
	EntityType EntityType
}

func (e EntityDestroyedEvent) EventType() string {
	return "EntityDestroyed"
}

type DamageDealtEvent struct {
	BaseEvent
	AttackerID EntityID
	TargetID   EntityID
	Damage     int32
	IsKilled   bool
}

func (e DamageDealtEvent) EventType() string {
	return "DamageDealt"
}

type ResourceMinedEvent struct {
	BaseEvent
	MinerID    EntityID
	AsteroidID EntityID
	Resource   ResourceType
	Amount     int32
}

func (e ResourceMinedEvent) EventType() string {
	return "ResourceMined"
}

type PlayerConnectedEvent struct {
	BaseEvent
	AccountID uint64
	Name      string
	SessionID string
	EntityID  EntityID
}

func (e PlayerConnectedEvent) EventType() string {
	return "PlayerConnected"
}

type PlayerDisconnectedEvent struct {
	BaseEvent
	AccountID uint64
	EntityID  EntityID
}

func (e PlayerDisconnectedEvent) EventType() string {
	return "PlayerDisconnected"
}
