package domain

import "hash/fnv"

type EntityID uint64

func HashNameToID(name string) EntityID {
	h := fnv.New64a()
	h.Write([]byte(name))
	return EntityID(90000 + h.Sum64()%1000000)
}

type EntityType uint8

const (
	EntityPlayer EntityType = iota
	EntityNPC
	EntityAsteroid
	EntityProjectile
	EntityStation
	EntityJumpGate
	EntityLootContainer
	EntityCombatMarker
	EntitySpaceBase
	EntityPlanet
	EntityMissile
)

func (e EntityType) String() string {
	switch e {
	case EntityPlayer:
		return "Player"
	case EntityNPC:
		return "NPC"
	case EntityAsteroid:
		return "Asteroid"
	case EntityProjectile:
		return "Projectile"
	case EntityStation:
		return "Station"
	case EntityJumpGate:
		return "JumpGate"
	case EntityLootContainer:
		return "LootContainer"
	case EntityCombatMarker:
		return "CombatMarker"
	case EntitySpaceBase:
		return "SpaceBase"
	case EntityPlanet:
		return "Planet"
	case EntityMissile:
		return "Missile"
	default:
		return "Unknown"
	}
}
