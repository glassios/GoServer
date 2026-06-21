package network

import (
	"fmt"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/systems"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

// BuildEntitySnapshot extracts entity state from World components into an EntitySnapshot.
func BuildEntitySnapshot(world *ecs.World, id domain.EntityID) *protocol.EntitySnapshot {
	eType, exists := world.GetEntityType(id)
	if !exists {
		return nil
	}

	snap := &protocol.EntitySnapshot{
		EntityId:   uint64(id),
		EntityType: uint32(eType),
	}

	// Transform
	if tVal, ok := world.GetComponent(id, domain.Transform{}); ok {
		t := tVal.(*domain.Transform)
		snap.X = t.X
		snap.Y = t.Y
		snap.Rotation = t.Rotation
	}

	// Velocity
	if vVal, ok := world.GetComponent(id, domain.Velocity{}); ok {
		v := vVal.(*domain.Velocity)
		snap.Vx = v.X
		snap.Vy = v.Y
	}

	// Health
	if hVal, ok := world.GetComponent(id, domain.Health{}); ok {
		h := hVal.(*domain.Health)
		snap.Hp = h.Current
		snap.MaxHp = h.Max
	}

	// Shield
	if sVal, ok := world.GetComponent(id, domain.Shield{}); ok {
		s := sVal.(*domain.Shield)
		snap.Shield = s.Current
		snap.MaxShield = s.Max
	}

	// Faction
	if fVal, ok := world.GetComponent(id, domain.FactionMember{}); ok {
		snap.FactionId = fVal.(*domain.FactionMember).FactionID
	}

	// Corporation
	if corpVal, ok := world.GetComponent(id, domain.CorporationMember{}); ok {
		snap.CorpId = corpVal.(*domain.CorporationMember).CorpID
	} else if ownerVal, ok := world.GetComponent(id, domain.StationOwnership{}); ok {
		stationCorpId := ownerVal.(*domain.StationOwnership).CorpID
		snap.FactionId = stationCorpId // The station's faction
		// Map station faction ID to the corresponding NPC corporation ID
		switch stationCorpId {
		case 1:
			snap.CorpId = 2 // Faction Pirates (1) -> Corp Pirates (2)
		case 2:
			snap.CorpId = 3 // Faction Miners (2) -> Corp Miners (3)
		case 3:
			snap.CorpId = 1 // Faction Galactic Guardians (3) -> Corp Galactic Guardians (1)
		}
	}

	// Weapon/Combat status
	if wVal, ok := world.GetComponent(id, domain.Weapon{}); ok {
		w := wVal.(*domain.Weapon)
		snap.TargetId = uint64(w.TargetID)
		// Only display shooting effects inside combat instances where entities have a CombatTeam
		_, hasTeam := world.GetComponent(id, domain.CombatTeam{})
		snap.IsShooting = w.IsFiring && hasTeam
	}

	// Mining status (overrides TargetId if mining is active)
	if mVal, ok := world.GetComponent(id, domain.MiningLaser{}); ok {
		m := mVal.(*domain.MiningLaser)
		if m.Active {
			snap.TargetId = uint64(m.TargetID)
			snap.IsMining = true
		}
	}

	// Combat Marker
	if cmVal, ok := world.GetComponent(id, domain.CombatMarker{}); ok {
		cm := cmVal.(*domain.CombatMarker)
		snap.TargetId = uint64(cm.CombatInstanceID)
		snap.Name = fmt.Sprintf("Battle Room %d", cm.CombatInstanceID)
		snap.QtyIron = int32(cm.AttackerID)
		snap.QtyTitanium = int32(cm.DefenderID)
	}

	// Name & Credits (PlayerData)
	if pVal, ok := world.GetComponent(id, domain.PlayerData{}); ok {
		pData := pVal.(*domain.PlayerData)
		snap.Name = pData.Name
		snap.Credits = pData.Credits
	} else if eType == domain.EntityNPC {
		snap.Name = "NPC"
		if cfgVal, ok := world.GetComponent(id, domain.ShipConfig{}); ok {
			shipType := cfgVal.(*domain.ShipConfig).ShipType
			switch shipType {
			case "miner":
				snap.Name = "NPC Miner"
			case "pirate":
				snap.Name = "NPC Pirate"
			case "patrol":
				snap.Name = "NPC Patrol"
			}
		}
	} else if eType == domain.EntityAsteroid {
		snap.Name = "Asteroid"
		if resVal, ok := world.GetComponent(id, domain.AsteroidResource{}); ok {
			res := resVal.(*domain.AsteroidResource)
			snap.Name = string(res.Type) + " Asteroid"
			snap.Hp = res.Amount
			snap.MaxHp = 1000 // Default initial seed amount
		}
	} else if eType == domain.EntityStation {
		snap.Name = "Station"
	} else if eType == domain.EntityLootContainer {
		snap.Name = "Loot Container"
	} else if eType == domain.EntitySpaceBase {
		snap.Name = "Звёздная база"
	}

	// Space base level (Phase 5)
	if baseVal, ok := world.GetComponent(id, domain.SpaceBase{}); ok {
		base := baseVal.(*domain.SpaceBase)
		snap.BaseLevel = base.Level
		snap.Name = fmt.Sprintf("База ур.%d", base.Level)
	}

	// Ship Config (for rendering/UI visuals)
	if cfgVal, ok := world.GetComponent(id, domain.ShipConfig{}); ok {
		snap.ShipType = cfgVal.(*domain.ShipConfig).ShipType
	}

	// Refinery Status
	if refVal, ok := world.GetComponent(id, domain.Refinery{}); ok {
		snap.RefineryActive = refVal.(*domain.Refinery).IsActive
	}

	// Shipyard Status
	if syVal, ok := world.GetComponent(id, domain.Shipyard{}); ok {
		snap.ShipyardQueueLen = int32(len(syVal.(*domain.Shipyard).Queue))
	}

	// Cargo items (Stock)
	if cargoVal, ok := world.GetComponent(id, domain.Cargo{}); ok {
		cargo := cargoVal.(*domain.Cargo)
		snap.QtyIron = cargo.GetResourceTypeQuantity(domain.ResourceIron)
		snap.QtyTitanium = cargo.GetResourceTypeQuantity(domain.ResourceTitanium)
		snap.QtyCrystal = cargo.GetResourceTypeQuantity(domain.ResourceCrystal)
		snap.QtyIronPlates = cargo.GetResourceTypeQuantity("IronPlates")
		snap.QtyTitaniumPlates = cargo.GetResourceTypeQuantity("TitaniumPlates")
		snap.QtySiliconWafers = cargo.GetResourceTypeQuantity(domain.ResourceSiliconWafers)
		snap.QtyFuelCells = cargo.GetResourceTypeQuantity(domain.ResourceFuelCells)
		snap.QtyMicrochips = cargo.GetResourceTypeQuantity(domain.ResourceMicrochips)
		snap.QtyEnergyCoils = cargo.GetResourceTypeQuantity(domain.ResourceEnergyCoils)
		snap.QtyLaserCannon = cargo.GetResourceTypeQuantity("Laser Cannon")
		snap.QtyMiningLaser = cargo.GetResourceTypeQuantity("Mining Laser")

		snap.CargoCapacity = cargo.Capacity
		var load int32
		for _, item := range cargo.Items {
			load += item.Quantity
		}
		snap.CargoLoad = load
	}

	// Loot Credits (overrides standard PlayerData credits if present)
	if lootVal, ok := world.GetComponent(id, domain.Loot{}); ok {
		snap.Credits = lootVal.(*domain.Loot).Credits
	}

	// Market Prices
	if marketVal, ok := world.GetComponent(id, domain.StationMarket{}); ok {
		market := marketVal.(*domain.StationMarket)
		if ironItem, exists := market.Items[domain.ResourceIron]; exists {
			snap.PriceIron = systems.CalculatePrice(ironItem, true)
		}
		if titaniumItem, exists := market.Items[domain.ResourceTitanium]; exists {
			snap.PriceTitanium = systems.CalculatePrice(titaniumItem, true)
		}
		if crystalItem, exists := market.Items[domain.ResourceCrystal]; exists {
			snap.PriceCrystal = systems.CalculatePrice(crystalItem, true)
		}
	}

	return snap
}

// BuildDeltaSnapshot compares visible entities with the previous tick and builds a DeltaSnapshot.
func BuildDeltaSnapshot(world *ecs.World, session *Session, tick uint64) (*protocol.DeltaSnapshot, bool) {
	playerID := session.GetEntityID()

	// Get current visible entities
	visVal, found := world.GetComponent(playerID, domain.Visibility{})
	if !found {
		return nil, false
	}
	visible := visVal.(*domain.Visibility).VisibleEntities

	// Get previously visible entities
	prevVisible := session.GetPreviouslyVisible()

	var updated []*protocol.EntitySnapshot
	var destroyed []uint64

	// 1. Find destroyed/out-of-sight entities
	for id := range prevVisible {
		if _, exists := visible[id]; !exists {
			destroyed = append(destroyed, uint64(id))
		}
	}

	// 2. Find new or updated entities
	// For simplicity in MVP, we send full state of all visible entities.
	// In production, we can optimize by only adding those that changed.
	for id := range visible {
		snap := BuildEntitySnapshot(world, id)
		if snap != nil {
			updated = append(updated, snap)
		}
	}

	// Always add the player itself to the snapshot
	playerSnap := BuildEntitySnapshot(world, playerID)
	if playerSnap != nil {
		updated = append(updated, playerSnap)
	}

	// Save visible entities for the next tick comparison
	session.SetPreviouslyVisible(visible)

	// If nothing changed and no entities are visible, we can skip sending (or send empty tick)
	if len(updated) == 0 && len(destroyed) == 0 {
		return &protocol.DeltaSnapshot{
			Tick: tick,
		}, true
	}

	return &protocol.DeltaSnapshot{
		Tick:              tick,
		UpdatedEntities:   updated,
		DestroyedEntities: destroyed,
	}, true
}

// BuildDeltaSnapshotFromWorldState compares a full WorldSnapshot with a session's previous state to build a DeltaSnapshot.
func BuildDeltaSnapshotFromWorldState(sysSnap *protocol.WorldSnapshot, session *Session, visibilityRadius float32) (*protocol.DeltaSnapshot, bool) {
	playerID := uint64(session.GetEntityID())

	// 1. Find player coordinates in the snapshot
	var playerSnap *protocol.EntitySnapshot
	for _, snap := range sysSnap.Entities {
		if snap.EntityId == playerID {
			playerSnap = snap
			break
		}
	}
	var px, py float32
	var r2 float32 = visibilityRadius * visibilityRadius

	if playerSnap != nil {
		px, py = playerSnap.X, playerSnap.Y
	} else {
		// If player flagship is destroyed but the fleet is still fighting,
		// we fallback to the origin and use infinite visibility so the player
		// continues to receive updates about the remaining ships.
		r2 = 999999.0 * 999999.0
	}

	// 2. Identify visible entities
	visible := make(map[domain.EntityID]struct{})
	var updated []*protocol.EntitySnapshot

	for _, snap := range sysSnap.Entities {
		// Calculate squared distance
		var dist2 float32
		if playerSnap != nil {
			dx := snap.X - px
			dy := snap.Y - py
			dist2 = dx*dx + dy*dy
		} else {
			dist2 = 0.0 // Always visible
		}

		if dist2 <= r2 || snap.EntityId == playerID {
			visible[domain.EntityID(snap.EntityId)] = struct{}{}
			updated = append(updated, snap)
		}
	}

	// 3. Compare with previously visible entities to find destroyed/out-of-sight ones
	prevVisible := session.GetPreviouslyVisible()
	var destroyed []uint64

	for id := range prevVisible {
		if _, exists := visible[id]; !exists {
			destroyed = append(destroyed, uint64(id))
		}
	}

	// Save visible entities for the next comparison
	session.SetPreviouslyVisible(visible)

	// Return delta snapshot
	return &protocol.DeltaSnapshot{
		Tick:              sysSnap.Tick,
		UpdatedEntities:   updated,
		DestroyedEntities: destroyed,
	}, true
}
