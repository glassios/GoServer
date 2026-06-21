package systems

import (
	"math"
	"math/rand"
	"time"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/pkg/mathutil"
)

type AISystem struct {
	random          *rand.Rand
	scanRange       float32
	accumulatedTime float64
	maxNPCs         int
	worldWidth      float32
	worldHeight     float32
	templates       []domain.NPCSnapshot
}

func NewAISystem(scanRange float32, maxNPCs int, width, height float32) *AISystem {
	return &AISystem{
		random:      rand.New(rand.NewSource(time.Now().UnixNano())),
		scanRange:   scanRange,
		maxNPCs:     maxNPCs,
		worldWidth:  width,
		worldHeight: height,
		templates:   nil,
	}
}

func (s *AISystem) SetNPCTemplates(templates []domain.NPCSnapshot) {
	s.templates = templates
}

func (s *AISystem) Name() string {
	return "AISystem"
}

func (s *AISystem) Priority() int {
	return 90 // Runs before movement and combat, setting up intents
}

func (s *AISystem) Update(world *ecs.World, dt float64) {
	s.accumulatedTime += dt

	s.spawnNPCsIfNeeded(world)

	mask := ecs.BuildMask(domain.Transform{}, domain.Velocity{}, domain.AIState{}, domain.ShipConfig{})
	entities := world.Query(mask)

	for _, id := range entities {
		tVal, _ := world.GetComponent(id, domain.Transform{})
		vVal, _ := world.GetComponent(id, domain.Velocity{})
		aiVal, _ := world.GetComponent(id, domain.AIState{})
		cfgVal, _ := world.GetComponent(id, domain.ShipConfig{})

		trans := tVal.(*domain.Transform)
		vel := vVal.(*domain.Velocity)
		ai := aiVal.(*domain.AIState)
		shipCfg := cfgVal.(*domain.ShipConfig)

		ai.StateTimer -= dt

		// Handle AI based on EntityType
		eType, exists := world.GetEntityType(id)
		if !exists {
			continue
		}

		myPos := mathutil.NewVec2(trans.X, trans.Y)


		switch eType {
		case domain.EntityNPC, domain.EntityPlayer:
			if _, inCombat := world.GetComponent(id, domain.CombatTeam{}); inCombat {
				ai.Behavior = domain.BehaviorAttack
			}

			switch ai.Behavior {
			case domain.BehaviorMine, domain.BehaviorIdle, domain.BehaviorDock:
				s.updateMiner(world, id, myPos, trans, vel, ai, shipCfg)
			case domain.BehaviorPatrol:
				s.updatePatrol(world, id, myPos, trans, vel, ai, shipCfg)
			case domain.BehaviorAttack:
				s.updateAttack(world, id, myPos, trans, vel, ai, shipCfg)
			case domain.BehaviorEscort:
				s.updateEscort(world, id, myPos, trans, vel, ai, shipCfg)
			case domain.BehaviorDefend:
				s.updateDefend(world, id, myPos, trans, vel, ai, shipCfg)
			default:
				s.updatePatrol(world, id, myPos, trans, vel, ai, shipCfg)
			}
		}
	}
}

func (s *AISystem) updateMiner(world *ecs.World, id domain.EntityID, myPos mathutil.Vec2, trans *domain.Transform, vel *domain.Velocity, ai *domain.AIState, shipCfg *domain.ShipConfig) {
	cVal, foundCargo := world.GetComponent(id, domain.Cargo{})
	laserVal, foundLaser := world.GetComponent(id, domain.MiningLaser{})
	if !foundCargo || !foundLaser {
		return
	}
	cargo := cVal.(*domain.Cargo)
	laser := laserVal.(*domain.MiningLaser)

	// Check if cargo is full
	currentVolume := int32(0)
	for _, item := range cargo.Items {
		currentVolume += item.Quantity
	}

	if currentVolume >= cargo.Capacity {
		ai.Behavior = domain.BehaviorDock
	}

	switch ai.Behavior {
	case domain.BehaviorIdle:
		// Find nearest asteroid
		asteroidID, _, found := s.findNearestEntity(world, myPos, domain.EntityAsteroid)
		if found {
			ai.Behavior = domain.BehaviorMine
			ai.TargetID = asteroidID
		} else {
			// No local asteroids, head to nearest Jump Gate if available
			_, gatePos, foundGate := s.findNearestEntity(world, myPos, domain.EntityJumpGate)
			if foundGate {
				dir := gatePos.Sub(myPos).Normalize()
				vel.X = dir.X * shipCfg.MaxSpeed
				vel.Y = dir.Y * shipCfg.MaxSpeed
			} else {
				// Wander slowly
				if ai.StateTimer <= 0 {
					ai.StateTimer = 3.0 + s.random.Float64()*3.0
					vel.X = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.3
					vel.Y = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.3
				}
			}
		}

	case domain.BehaviorMine:
		// Check target existence
		targetPos, targetExists := s.getEntityPos(world, ai.TargetID)
		if !targetExists {
			laser.Active = false
			ai.Behavior = domain.BehaviorIdle
			return
		}

		dist := myPos.Distance(targetPos)
		if dist <= laser.Range {
			// Stop and mine
			vel.X = 0
			vel.Y = 0
			laser.Active = true
			laser.TargetID = ai.TargetID
		} else {
			// Move to asteroid
			laser.Active = false
			dir := targetPos.Sub(myPos).Normalize()
			vel.X = dir.X * shipCfg.MaxSpeed
			vel.Y = dir.Y * shipCfg.MaxSpeed
		}

	case domain.BehaviorDock:
		laser.Active = false
		
		var myFactionID uint32 = 0
		if fVal, ok := world.GetComponent(id, domain.FactionMember{}); ok {
			myFactionID = fVal.(*domain.FactionMember).FactionID
		}

		// Find nearest station belonging to our faction
		stationID, stationPos, found := s.findNearestFactionStation(world, myPos, myFactionID)
		if !found {
			// Fallback to any station
			stationID, stationPos, found = s.findNearestEntity(world, myPos, domain.EntityStation)
		}

		if !found {
			// No local stations, head to nearest Jump Gate if available
			_, gatePos, foundGate := s.findNearestEntity(world, myPos, domain.EntityJumpGate)
			if foundGate {
				dir := gatePos.Sub(myPos).Normalize()
				vel.X = dir.X * shipCfg.MaxSpeed
				vel.Y = dir.Y * shipCfg.MaxSpeed
			} else {
				ai.Behavior = domain.BehaviorIdle
			}
			return
		}

		dist := myPos.Distance(stationPos)
		if dist <= 100.0 {
			// Arrived at station, deposit cargo to corporate vault
			if vaultVal, ok := world.GetComponent(stationID, domain.CorporationVault{}); ok {
				vault := vaultVal.(*domain.CorporationVault)
				for _, item := range cargo.Items {
					foundItem := false
					for i, vItem := range vault.Items {
						if vItem.DefinitionID == item.DefinitionID {
							vault.Items[i].Quantity += item.Quantity
							foundItem = true
							break
						}
					}
					if !foundItem {
						vault.Items = append(vault.Items, domain.ItemInstance{
							DefinitionID: item.DefinitionID,
							Quantity:     item.Quantity,
							LocationType: "STATION_CORP_VAULT",
							LocationID:   uint64(stationID),
							OwnerID:      uint64(vault.OwnerCorpID),
							State:        "normal",
						})
					}
				}
			}
			cargo.Items = []domain.ItemInstance{} // Clear cargo
			ai.Behavior = domain.BehaviorIdle
			ai.TargetID = 0
		} else {
			// Move to station
			dir := stationPos.Sub(myPos).Normalize()
			vel.X = dir.X * shipCfg.MaxSpeed
			vel.Y = dir.Y * shipCfg.MaxSpeed
		}
	}
}

func (s *AISystem) updatePirate(world *ecs.World, id domain.EntityID, myPos mathutil.Vec2, trans *domain.Transform, vel *domain.Velocity, ai *domain.AIState, shipCfg *domain.ShipConfig) {
	// Combat is disabled. Just patrol around home position.
	homeDist := myPos.Distance(mathutil.NewVec2(ai.HomePos.X, ai.HomePos.Y))
	if homeDist > 1500 {
		homePos := mathutil.NewVec2(ai.HomePos.X, ai.HomePos.Y)
		dir := homePos.Sub(myPos).Normalize()
		vel.X = dir.X * shipCfg.MaxSpeed * 0.5
		vel.Y = dir.Y * shipCfg.MaxSpeed * 0.5
	} else if ai.StateTimer <= 0 {
		ai.StateTimer = 4.0 + s.random.Float64()*4.0
		vel.X = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.5
		vel.Y = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.5
	}
}

func (s *AISystem) updatePatrol(world *ecs.World, id domain.EntityID, myPos mathutil.Vec2, trans *domain.Transform, vel *domain.Velocity, ai *domain.AIState, shipCfg *domain.ShipConfig) {
	var factionID uint32 = 0
	if fVal, ok := world.GetComponent(id, domain.FactionMember{}); ok {
		factionID = fVal.(*domain.FactionMember).FactionID
	}

	// 1. Проверяем текущую цель преследования, если она есть
	hasTarget := false
	if ai.TargetID != 0 {
		// Проверим, существует ли цель, не в бою ли она, и находится ли она на расстоянии <= 500
		targetTVal, targetExists := world.GetComponent(ai.TargetID, domain.Transform{})
		_, targetInCombat := world.GetComponent(ai.TargetID, domain.CombatState{})
		
		if targetExists && !targetInCombat {
			t := targetTVal.(*domain.Transform)
			targetPos := mathutil.NewVec2(t.X, t.Y)
			dist := myPos.Distance(targetPos)
			if dist <= 500.0 {
				hasTarget = true
			}
		}
		
		if !hasTarget {
			// Цель утеряна или вышла из зоны преследования
			ai.TargetID = 0
		}
	}

	// Если у нас нет текущей цели (или она только что была сброшена), ищем новую ближайшую
	if ai.TargetID == 0 {
		var nearestEnemyID domain.EntityID
		minEnemyDist := float32(500.0)
		foundEnemy := false

		allEntities := world.Query(ecs.BuildMask(domain.Transform{}))

		for _, entID := range allEntities {
			if entID == id {
				continue
			}

			// Пропускаем тех, кто уже в бою
			if _, inCombat := world.GetComponent(entID, domain.CombatState{}); inCombat {
				continue
			}

			eType, exists := world.GetEntityType(entID)
			if !exists {
				continue
			}

			isEnemy := false

			if factionID == 1 {
				// Пираты враждебны ко всем, кроме других пиратов NPC
				var targetFaction uint32 = 0
				if fVal, ok := world.GetComponent(entID, domain.FactionMember{}); ok {
					targetFaction = fVal.(*domain.FactionMember).FactionID
				}

				// Враждебны: игроки и любые NPC, кроме пиратов
				if eType == domain.EntityPlayer {
					isEnemy = true
				} else if eType == domain.EntityNPC && targetFaction != 1 {
					isEnemy = true
				}
			} else if factionID == 2 {
				// Патрули враждебны только к пиратам (FactionID == 1) и преступникам (MinerAttacker)
				var targetFaction uint32 = 0
				if fVal, ok := world.GetComponent(entID, domain.FactionMember{}); ok {
					targetFaction = fVal.(*domain.FactionMember).FactionID
				}
				_, isCrim := world.GetComponent(entID, domain.MinerAttacker{})

				if targetFaction == 1 || isCrim {
					isEnemy = true
				}
			}

			if isEnemy {
				tVal, _ := world.GetComponent(entID, domain.Transform{})
				t := tVal.(*domain.Transform)
				pos := mathutil.NewVec2(t.X, t.Y)
				dist := myPos.Distance(pos)
				if dist < minEnemyDist {
					minEnemyDist = dist
					nearestEnemyID = entID
					foundEnemy = true
				}
			}
		}

		if foundEnemy {
			ai.TargetID = nearestEnemyID
			hasTarget = true
		}
	}

	// Если цель валидна — летим к ней
	if hasTarget && ai.TargetID != 0 {
		targetTVal, _ := world.GetComponent(ai.TargetID, domain.Transform{})
		t := targetTVal.(*domain.Transform)
		targetPos := mathutil.NewVec2(t.X, t.Y)
		dir := targetPos.Sub(myPos).Normalize()
		vel.X = dir.X * shipCfg.MaxSpeed
		vel.Y = dir.Y * shipCfg.MaxSpeed
		if vel.X != 0 || vel.Y != 0 {
			trans.Rotation = float32(math.Atan2(float64(vel.Y), float64(vel.X)))
		}
		return
	}

	// 2. Если рядом есть активный бой (CombatMarker), летим в его сторону (только для патрулей)
	if factionID == 2 {
		mask := ecs.BuildMask(domain.Transform{}, domain.CombatMarker{})
		markers := world.Query(mask)
		
		var nearestMarkerPos mathutil.Vec2
		minDist := float32(500.0) // Ищем в радиусе 500
		foundMarker := false
		
		for _, markerID := range markers {
			tVal, _ := world.GetComponent(markerID, domain.Transform{})
			t := tVal.(*domain.Transform)
			pos := mathutil.NewVec2(t.X, t.Y)
			dist := myPos.Distance(pos)
			if dist < minDist {
				minDist = dist
				nearestMarkerPos = pos
				foundMarker = true
			}
		}
		
		if foundMarker {
			dir := nearestMarkerPos.Sub(myPos).Normalize()
			vel.X = dir.X * shipCfg.MaxSpeed
			vel.Y = dir.Y * shipCfg.MaxSpeed
			if vel.X != 0 || vel.Y != 0 {
				trans.Rotation = float32(math.Atan2(float64(vel.Y), float64(vel.X)))
			}
			return
		}
	}

	// 3. Иначе просто патрулируем вокруг home position
	homeDist := myPos.Distance(mathutil.NewVec2(ai.HomePos.X, ai.HomePos.Y))
	if homeDist > 2000 {
		homePos := mathutil.NewVec2(ai.HomePos.X, ai.HomePos.Y)
		dir := homePos.Sub(myPos).Normalize()
		vel.X = dir.X * shipCfg.MaxSpeed * 0.6
		vel.Y = dir.Y * shipCfg.MaxSpeed * 0.6
	} else if ai.StateTimer <= 0 {
		ai.StateTimer = 5.0 + s.random.Float64()*5.0
		vel.X = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.6
		vel.Y = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.6
		if vel.X != 0 || vel.Y != 0 {
			trans.Rotation = float32(math.Atan2(float64(vel.Y), float64(vel.X)))
		}
	}
}

// Helper methods

func (s *AISystem) findNearestEntity(world *ecs.World, myPos mathutil.Vec2, eType domain.EntityType) (domain.EntityID, mathutil.Vec2, bool) {
	mask := ecs.BuildMask(domain.Transform{})
	entities := world.Query(mask)

	var nearestID domain.EntityID
	var nearestPos mathutil.Vec2
	minDist := float32(math.MaxFloat32)
	found := false

	for _, id := range entities {
		t, ok := world.GetEntityType(id)
		if !ok || t != eType {
			continue
		}

		tVal, _ := world.GetComponent(id, domain.Transform{})
		trans := tVal.(*domain.Transform)
		pos := mathutil.NewVec2(trans.X, trans.Y)

		dist := myPos.Distance(pos)
		if dist < minDist {
			minDist = dist
			nearestID = id
			nearestPos = pos
			found = true
		}
	}
	return nearestID, nearestPos, found
}

func (s *AISystem) findNearestPirate(world *ecs.World, myPos mathutil.Vec2) (domain.EntityID, mathutil.Vec2, bool) {
	mask := ecs.BuildMask(domain.Transform{}, domain.FactionMember{})
	entities := world.Query(mask)

	var nearestID domain.EntityID
	var nearestPos mathutil.Vec2
	minDist := float32(math.MaxFloat32)
	found := false

	for _, id := range entities {
		t, ok := world.GetEntityType(id)
		if !ok || t != domain.EntityNPC {
			continue
		}

		fVal, _ := world.GetComponent(id, domain.FactionMember{})
		if fVal.(*domain.FactionMember).FactionID != 1 { // Not a pirate
			continue
		}

		tVal, _ := world.GetComponent(id, domain.Transform{})
		trans := tVal.(*domain.Transform)
		pos := mathutil.NewVec2(trans.X, trans.Y)

		dist := myPos.Distance(pos)
		if dist < minDist {
			minDist = dist
			nearestID = id
			nearestPos = pos
			found = true
		}
	}
	return nearestID, nearestPos, found
}

func (s *AISystem) getEntityPos(world *ecs.World, id domain.EntityID) (mathutil.Vec2, bool) {
	tVal, found := world.GetComponent(id, domain.Transform{})
	if !found {
		return mathutil.Vec2{}, false
	}
	trans := tVal.(*domain.Transform)
	return mathutil.NewVec2(trans.X, trans.Y), true
}

func (s *AISystem) spawnNPCsIfNeeded(world *ecs.World) {
	if len(s.templates) > 0 {
		// Predefined database NPCs mode
		for _, t := range s.templates {
			if _, exists := world.GetEntityType(t.EntityID); !exists {
				// Respawn this specific NPC fleet!
				world.RegisterEntityWithID(t.EntityID, domain.EntityNPC)
				world.AddComponent(t.EntityID, &domain.Transform{X: t.Transform.X, Y: t.Transform.Y})
				world.AddComponent(t.EntityID, &domain.Velocity{X: 0, Y: 0})
				behavior := domain.BehaviorIdle
				if t.Behavior != "" {
					behavior = domain.AIBehavior(t.Behavior)
				}
				world.AddComponent(t.EntityID, &domain.AIState{
					Behavior: behavior,
					HomePos:  domain.Transform{X: t.Transform.X, Y: t.Transform.Y},
				})

				// Calculate total cargo capacity of the NPC fleet
				var totalCargoCapacity int32 = 0
				for _, sh := range t.Ships {
					totalCargoCapacity += sh.CargoCapacity
				}

				// The first ship's configuration acts as the main ship config for movement/vis
				var maxSpeed float32 = 50.0
				var shipType string = "fighter"
				if len(t.Ships) > 0 {
					shipType = t.Ships[0].ShipType
					switch shipType {
					case "miner":
						maxSpeed = 40.0
					case "pirate":
						maxSpeed = 50.0
					case "patrol":
						maxSpeed = 60.0
					}
				}

				world.AddComponent(t.EntityID, &domain.ShipConfig{
					ShipType: shipType,
					MaxSpeed: maxSpeed,
					TurnRate: 1.5,
				})
				world.AddComponent(t.EntityID, &domain.Health{
					Current: t.Ships[0].Health,
					Max:     t.Ships[0].MaxHealth,
				})
				world.AddComponent(t.EntityID, &domain.Shield{
					Current:   t.Ships[0].Shield,
					Max:       t.Ships[0].MaxShield,
					RegenRate: 1.0,
				})

				// If any ship is a miner, enable mining laser
				isMiner := false
				for _, sh := range t.Ships {
					if sh.ShipType == "miner" {
						isMiner = true
						break
					}
				}

				if isMiner {
					world.AddComponent(t.EntityID, &domain.MiningLaser{Power: 5, Range: 80})
					world.AddComponent(t.EntityID, &domain.PlayerData{Name: t.Name, Credits: 0})
				} else {
					// Add dummy weapon so weapon component checks pass, but combat is disabled anyway
					var weaponRange float32 = 150.0
					if shipType == "patrol" {
						weaponRange = 180.0
					}
					weaponRange = weaponRange / 3.0

					world.AddComponent(t.EntityID, &domain.Weapon{
						Type:     domain.WeaponLaser,
						Damage:   6,
						Range:    weaponRange,
						Cooldown: 1.2,
					})
				}

				// Unified fleet cargo
				world.AddComponent(t.EntityID, &domain.Cargo{
					Items:    []domain.ItemInstance{},
					Capacity: totalCargoCapacity,
				})

				world.AddComponent(t.EntityID, &domain.FactionMember{FactionID: t.FactionID})
				if t.CorpID > 0 {
					world.AddComponent(t.EntityID, &domain.CorporationMember{CorpID: t.CorpID, Role: "Member"})
				}

				// Fleet component
				var fleetShips []domain.FleetShip
				for _, sh := range t.Ships {
					fleetShips = append(fleetShips, domain.FleetShip{
						ShipID:        sh.ShipID,
						ShipType:      sh.ShipType,
						Health:        sh.Health,
						MaxHealth:     sh.MaxHealth,
						Shield:        sh.Shield,
						MaxShield:     sh.MaxShield,
						CargoCapacity: sh.CargoCapacity,
					})
				}
				world.AddComponent(t.EntityID, &domain.Fleet{
					Ships: fleetShips,
				})
			}
		}
		return
	}

	// Fallback to random spawning (with Fleet components too!)
	mask := ecs.BuildMask(domain.AIState{})
	npcEntities := world.Query(mask)
	currentCount := len(npcEntities)

	if currentCount >= s.maxNPCs {
		return
	}

	needed := s.maxNPCs - currentCount

	for i := 0; i < needed; i++ {
		npc := world.CreateEntity(domain.EntityNPC)
		x := (s.random.Float32() - 0.5) * s.worldWidth
		y := (s.random.Float32() - 0.5) * s.worldHeight

		world.AddComponent(npc, &domain.Transform{X: x, Y: y})
		world.AddComponent(npc, &domain.Velocity{X: 0, Y: 0})
		aiState := &domain.AIState{
			Behavior: domain.BehaviorIdle,
			HomePos:  domain.Transform{X: x, Y: y},
		}
		world.AddComponent(npc, aiState)

		// 60% Miner, 20% Pirate, 20% Patrol
		r := s.random.Float64()
		if r < 0.6 {
			aiState.Behavior = domain.BehaviorIdle
			// Miner fleet (1 miner ship + 1 cargo helper)
			ships := []domain.FleetShip{
				{ShipID: 1, ShipType: "miner", Health: 80, MaxHealth: 80, Shield: 30, MaxShield: 30, CargoCapacity: 150},
				{ShipID: 2, ShipType: "cargo_helper", Health: 100, MaxHealth: 100, Shield: 40, MaxShield: 40, CargoCapacity: 200},
			}
			world.AddComponent(npc, &domain.Fleet{Ships: ships})
			world.AddComponent(npc, &domain.ShipConfig{ShipType: "miner", MaxSpeed: 40})
			world.AddComponent(npc, &domain.Health{Current: 80, Max: 80})
			world.AddComponent(npc, &domain.Shield{Current: 30, Max: 30, RegenRate: 1.0})
			world.AddComponent(npc, &domain.Cargo{Items: []domain.ItemInstance{}, Capacity: 350})
			world.AddComponent(npc, &domain.MiningLaser{Power: 5, Range: 80})
			world.AddComponent(npc, &domain.FactionMember{FactionID: 2})
			world.AddComponent(npc, &domain.PlayerData{Name: "NPC Miner", Credits: 0})
		} else if r < 0.8 {
			// Pirate fleet
			pirateRoll := s.random.Float64()
			if pirateRoll < 0.6 {
				// Pirate Miner (1 miner ship + 1 cargo helper)
				aiState.Behavior = domain.BehaviorIdle
				ships := []domain.FleetShip{
					{ShipID: 1, ShipType: "miner", Health: 80, MaxHealth: 80, Shield: 30, MaxShield: 30, CargoCapacity: 150},
					{ShipID: 2, ShipType: "cargo_helper", Health: 100, MaxHealth: 100, Shield: 40, MaxShield: 40, CargoCapacity: 200},
				}
				world.AddComponent(npc, &domain.Fleet{Ships: ships})
				world.AddComponent(npc, &domain.ShipConfig{ShipType: "miner", MaxSpeed: 40})
				world.AddComponent(npc, &domain.Health{Current: 80, Max: 80})
				world.AddComponent(npc, &domain.Shield{Current: 30, Max: 30, RegenRate: 1.0})
				world.AddComponent(npc, &domain.Cargo{Items: []domain.ItemInstance{}, Capacity: 350})
				world.AddComponent(npc, &domain.MiningLaser{Power: 5, Range: 80})
				world.AddComponent(npc, &domain.FactionMember{FactionID: 1})
				world.AddComponent(npc, &domain.PlayerData{Name: "Pirate Miner", Credits: 0})
			} else if pirateRoll < 0.8 {
				// Pirate Patrol
				aiState.Behavior = domain.BehaviorPatrol
				ships := []domain.FleetShip{
					{ShipID: 1, ShipType: "pirate", Health: 60, MaxHealth: 60, Shield: 20, MaxShield: 20, CargoCapacity: 50},
				}
				world.AddComponent(npc, &domain.Fleet{Ships: ships})
				world.AddComponent(npc, &domain.ShipConfig{ShipType: "pirate", MaxSpeed: 50})
				world.AddComponent(npc, &domain.Health{Current: 60, Max: 60})
				world.AddComponent(npc, &domain.Shield{Current: 20, Max: 20, RegenRate: 1.0})
				world.AddComponent(npc, &domain.Cargo{Items: []domain.ItemInstance{}, Capacity: 50})
				world.AddComponent(npc, &domain.Weapon{Type: domain.WeaponLaser, Damage: 6, Range: 50, Cooldown: 1.2})
				world.AddComponent(npc, &domain.FactionMember{FactionID: 1})
			} else {
				// Pirate Escort
				aiState.Behavior = domain.BehaviorEscort
				ships := []domain.FleetShip{
					{ShipID: 1, ShipType: "pirate", Health: 60, MaxHealth: 60, Shield: 20, MaxShield: 20, CargoCapacity: 50},
				}
				world.AddComponent(npc, &domain.Fleet{Ships: ships})
				world.AddComponent(npc, &domain.ShipConfig{ShipType: "pirate", MaxSpeed: 50})
				world.AddComponent(npc, &domain.Health{Current: 60, Max: 60})
				world.AddComponent(npc, &domain.Shield{Current: 20, Max: 20, RegenRate: 1.0})
				world.AddComponent(npc, &domain.Cargo{Items: []domain.ItemInstance{}, Capacity: 50})
				world.AddComponent(npc, &domain.Weapon{Type: domain.WeaponLaser, Damage: 6, Range: 50, Cooldown: 1.2})
				world.AddComponent(npc, &domain.FactionMember{FactionID: 1})
			}
		} else {
			// Patrol fleet
			patrolRoll := s.random.Float64()
			if patrolRoll < 0.5 {
				aiState.Behavior = domain.BehaviorPatrol
			} else {
				aiState.Behavior = domain.BehaviorDefend
			}
			ships := []domain.FleetShip{
				{ShipID: 1, ShipType: "patrol", Health: 100, MaxHealth: 100, Shield: 50, MaxShield: 50, CargoCapacity: 100},
			}
			world.AddComponent(npc, &domain.Fleet{Ships: ships})
			world.AddComponent(npc, &domain.ShipConfig{ShipType: "patrol", MaxSpeed: 60})
			world.AddComponent(npc, &domain.Health{Current: 100, Max: 100})
			world.AddComponent(npc, &domain.Shield{Current: 50, Max: 50, RegenRate: 1.0})
			world.AddComponent(npc, &domain.Cargo{Items: []domain.ItemInstance{}, Capacity: 100})
			world.AddComponent(npc, &domain.Weapon{Type: domain.WeaponPlasma, Damage: 8, Range: 60, Cooldown: 1.5})
			world.AddComponent(npc, &domain.FactionMember{FactionID: 2})
		}
	}
}

func (s *AISystem) updateAttack(world *ecs.World, id domain.EntityID, myPos mathutil.Vec2, trans *domain.Transform, vel *domain.Velocity, ai *domain.AIState, shipCfg *domain.ShipConfig) {
	// Проверяем, есть ли компонент боевой команды (значит мы в инстансе боя)
	myTeamVal, hasTeam := world.GetComponent(id, domain.CombatTeam{})
	if !hasTeam {
		// Старое поведение: патрулирование вокруг HomePos
		homeDist := myPos.Distance(mathutil.NewVec2(ai.HomePos.X, ai.HomePos.Y))
		if homeDist > 1500 {
			homePos := mathutil.NewVec2(ai.HomePos.X, ai.HomePos.Y)
			dir := homePos.Sub(myPos).Normalize()
			vel.X = dir.X * shipCfg.MaxSpeed * 0.5
			vel.Y = dir.Y * shipCfg.MaxSpeed * 0.5
		} else if ai.StateTimer <= 0 {
			ai.StateTimer = 4.0 + s.random.Float64()*4.0
			vel.X = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.5
			vel.Y = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.5
		}
		return
	}

	myTeamID := myTeamVal.(*domain.CombatTeam).TeamID

	// Phase 1.5: role & stance drive positioning and target priority.
	role := domain.RoleDPS
	if rVal, ok := world.GetComponent(id, domain.CombatRole{}); ok {
		role = rVal.(*domain.CombatRole).Role
	}
	stance := domain.StanceAttack
	if stVal, ok := world.GetComponent(id, domain.CombatStrategy{}); ok {
		stance = stVal.(*domain.CombatStrategy).Stance
	}

	wVal, hasWeapon := world.GetComponent(id, domain.Weapon{})
	targetID, targetPos, dist, found := selectCombatTarget(world, id, myTeamID, myPos)

	if !found {
		if hasWeapon {
			wVal.(*domain.Weapon).Active = false
		}
		ai.TargetID = 0
		vel.X = 0
		vel.Y = 0
		return
	}

	ai.TargetID = targetID
	weaponRange := float32(300)
	if hasWeapon {
		weapon := wVal.(*domain.Weapon)
		weapon.TargetID = targetID
		weapon.Active = true
		if weapon.Range > 0 {
			weaponRange = weapon.Range
		}
	}

	dirToTarget := targetPos.Sub(myPos)
	if dirToTarget.Length() > 0 {
		dirToTarget = dirToTarget.Normalize()
	}
	facing := float32(math.Atan2(float64(dirToTarget.Y), float64(dirToTarget.X)))

	// Retreat: kite directly away from the threat (weapon stays active to fire while fleeing).
	if stance == domain.StanceRetreat {
		away := myPos.Sub(targetPos)
		if away.Length() > 0 {
			away = away.Normalize()
		}
		vel.X = away.X * shipCfg.MaxSpeed
		vel.Y = away.Y * shipCfg.MaxSpeed
		trans.Rotation = facing
		return
	}

	// Role-based preferred engagement band (fraction of weapon range).
	minFrac, maxFrac := float32(0.7), float32(0.9)
	switch role {
	case domain.RoleTank:
		minFrac, maxFrac = 0.4, 0.65 // hold the front line, draw fire
	case domain.RoleSupport, domain.RoleRepair:
		minFrac, maxFrac = 0.9, 1.15 // stay at the back
	}
	minR := weaponRange * minFrac
	maxR := weaponRange * maxFrac

	// Defensive stance holds ground until the enemy closes in.
	if stance == domain.StanceDefense && dist > weaponRange*1.25 {
		vel.X = 0
		vel.Y = 0
		trans.Rotation = facing
		return
	}

	if dist > maxR {
		vel.X = dirToTarget.X * shipCfg.MaxSpeed
		vel.Y = dirToTarget.Y * shipCfg.MaxSpeed
	} else if dist < minR {
		vel.X = -dirToTarget.X * shipCfg.MaxSpeed * 0.4
		vel.Y = -dirToTarget.Y * shipCfg.MaxSpeed * 0.4
	} else {
		vel.X = 0
		vel.Y = 0
	}
	trans.Rotation = facing
}

// selectCombatTarget picks an enemy by a threat score that yields natural focus fire: the
// weakest enemy (lowest hull+armor+shield) is preferred, closer enemies are weighted higher,
// and ships in the tank role get a "taunt" bonus so they draw fire from the front line.
func selectCombatTarget(world *ecs.World, selfID domain.EntityID, myTeamID uint32, myPos mathutil.Vec2) (domain.EntityID, mathutil.Vec2, float32, bool) {
	entities := world.Query(ecs.BuildMask(domain.Transform{}, domain.Health{}, domain.CombatTeam{}))

	var bestID domain.EntityID
	var bestPos mathutil.Vec2
	var bestDist float32
	bestScore := float32(math.MaxFloat32)
	found := false

	for _, entID := range entities {
		if entID == selfID {
			continue
		}
		teamVal, _ := world.GetComponent(entID, domain.CombatTeam{})
		if teamVal.(*domain.CombatTeam).TeamID == myTeamID {
			continue
		}
		hVal, _ := world.GetComponent(entID, domain.Health{})
		h := hVal.(*domain.Health)
		if h.Current <= 0 {
			continue
		}
		tVal, _ := world.GetComponent(entID, domain.Transform{})
		t := tVal.(*domain.Transform)
		pos := mathutil.NewVec2(t.X, t.Y)
		dist := myPos.Distance(pos)

		effHP := float32(h.Current)
		if aVal, ok := world.GetComponent(entID, domain.ArmorGrid{}); ok {
			effHP += aVal.(*domain.ArmorGrid).Current
		}
		if sVal, ok := world.GetComponent(entID, domain.Shield{}); ok {
			effHP += float32(sVal.(*domain.Shield).Current)
		}

		score := effHP*0.5 + dist
		if rVal, ok := world.GetComponent(entID, domain.CombatRole{}); ok {
			if rVal.(*domain.CombatRole).Role == domain.RoleTank {
				score -= 400 // taunt: tanks are more attractive targets
			}
		}

		if score < bestScore {
			bestScore = score
			bestID = entID
			bestPos = pos
			bestDist = dist
			found = true
		}
	}
	return bestID, bestPos, bestDist, found
}

func (s *AISystem) updateEscort(world *ecs.World, id domain.EntityID, myPos mathutil.Vec2, trans *domain.Transform, vel *domain.Velocity, ai *domain.AIState, shipCfg *domain.ShipConfig) {
	// Проверяем, есть ли боевая команда (значит мы в инстансе боя)
	myTeamVal, hasTeam := world.GetComponent(id, domain.CombatTeam{})
	if !hasTeam {
		// Обычное поведение вне инстанса боя: летим за лидером своей фракции
		var myFactionID uint32 = 0
		if fVal, ok := world.GetComponent(id, domain.FactionMember{}); ok {
			myFactionID = fVal.(*domain.FactionMember).FactionID
		}

		if ai.TargetID != 0 {
			targetTVal, targetExists := world.GetComponent(ai.TargetID, domain.Transform{})
			var targetFactionID uint32 = 0
			if targetExists {
				if tfVal, ok := world.GetComponent(ai.TargetID, domain.FactionMember{}); ok {
					targetFactionID = tfVal.(*domain.FactionMember).FactionID
				}
			}
			if !targetExists || targetFactionID != myFactionID {
				ai.TargetID = 0
			} else {
				targetTrans := targetTVal.(*domain.Transform)
				targetPos := mathutil.NewVec2(targetTrans.X, targetTrans.Y)
				dist := myPos.Distance(targetPos)

				if dist > 200.0 {
					dir := targetPos.Sub(myPos).Normalize()
					vel.X = dir.X * shipCfg.MaxSpeed
					vel.Y = dir.Y * shipCfg.MaxSpeed
				} else if dist > 100.0 {
					dir := targetPos.Sub(myPos).Normalize()
					vel.X = dir.X * shipCfg.MaxSpeed * 0.5
					vel.Y = dir.Y * shipCfg.MaxSpeed * 0.5
				} else {
					vel.X = 0
					vel.Y = 0
				}
				return
			}
		}

		mask := ecs.BuildMask(domain.Transform{}, domain.FactionMember{}, domain.Fleet{})
		entities := world.Query(mask)

		var nearestID domain.EntityID
		var nearestPos mathutil.Vec2
		minDist := float32(math.MaxFloat32)
		found := false

		for _, entID := range entities {
			if entID == id {
				continue
			}
			fVal, _ := world.GetComponent(entID, domain.FactionMember{})
			if fVal.(*domain.FactionMember).FactionID != myFactionID {
				continue
			}

			tVal, _ := world.GetComponent(entID, domain.Transform{})
			t := tVal.(*domain.Transform)
			pos := mathutil.NewVec2(t.X, t.Y)
			dist := myPos.Distance(pos)

			if dist < minDist {
				minDist = dist
				nearestID = entID
				nearestPos = pos
				found = true
			}
		}

		if found {
			ai.TargetID = nearestID
			dir := nearestPos.Sub(myPos).Normalize()
			vel.X = dir.X * shipCfg.MaxSpeed
			vel.Y = dir.Y * shipCfg.MaxSpeed
		} else {
			if ai.StateTimer <= 0 {
				ai.StateTimer = 4.0 + s.random.Float64()*4.0
				vel.X = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.5
				vel.Y = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.5
			}
		}
		return
	}

	myTeamID := myTeamVal.(*domain.CombatTeam).TeamID

	// 1. Поведение следования за лидером флагманом внутри инстанса боя
	leaderID := ai.TargetID
	leaderExists := false
	var leaderPos mathutil.Vec2

	if leaderID != 0 {
		if tVal, ok := world.GetComponent(leaderID, domain.Transform{}); ok {
			transL := tVal.(*domain.Transform)
			leaderPos = mathutil.NewVec2(transL.X, transL.Y)
			leaderExists = true
		}
	}

	if leaderExists {
		dist := myPos.Distance(leaderPos)
		if dist > 150.0 {
			dir := leaderPos.Sub(myPos).Normalize()
			vel.X = dir.X * shipCfg.MaxSpeed
			vel.Y = dir.Y * shipCfg.MaxSpeed
			trans.Rotation = float32(math.Atan2(float64(dir.Y), float64(dir.X)))
		} else if dist > 60.0 {
			dir := leaderPos.Sub(myPos).Normalize()
			vel.X = dir.X * shipCfg.MaxSpeed * 0.4
			vel.Y = dir.Y * shipCfg.MaxSpeed * 0.4
			trans.Rotation = float32(math.Atan2(float64(dir.Y), float64(dir.X)))
		} else {
			vel.X = 0
			vel.Y = 0
		}
	} else {
		// Лидер уничтожен, переходим в режим самостоятельной атаки
		ai.Behavior = domain.BehaviorAttack
		ai.TargetID = 0
		return
	}

	// 2. Автоматический поиск и обстрел противников
	wVal, hasWeapon := world.GetComponent(id, domain.Weapon{})
	if hasWeapon {
		weapon := wVal.(*domain.Weapon)

		var nearestEnemyID domain.EntityID
		var nearestEnemyPos mathutil.Vec2
		minDist := float32(math.MaxFloat32)
		enemyFound := false

		mask := ecs.BuildMask(domain.Transform{}, domain.Health{}, domain.CombatTeam{})
		entities := world.Query(mask)

		for _, entID := range entities {
			if entID == id {
				continue
			}
			teamVal, _ := world.GetComponent(entID, domain.CombatTeam{})
			teamID := teamVal.(*domain.CombatTeam).TeamID

			if teamID == myTeamID {
				continue
			}

			hVal, _ := world.GetComponent(entID, domain.Health{})
			health := hVal.(*domain.Health)
			if health.Current <= 0 {
				continue
			}

			tVal, _ := world.GetComponent(entID, domain.Transform{})
			t := tVal.(*domain.Transform)
			pos := mathutil.NewVec2(t.X, t.Y)
			dist := myPos.Distance(pos)

			if dist < minDist {
				minDist = dist
				nearestEnemyID = entID
				nearestEnemyPos = pos
				enemyFound = true
			}
		}

		if enemyFound && minDist <= weapon.Range {
			weapon.TargetID = nearestEnemyID
			weapon.Active = true
			// Направляем нос на противника при стрельбе
			dir := nearestEnemyPos.Sub(myPos).Normalize()
			trans.Rotation = float32(math.Atan2(float64(dir.Y), float64(dir.X)))
		} else {
			weapon.Active = false
		}
	}
}

func (s *AISystem) updateDefend(world *ecs.World, id domain.EntityID, myPos mathutil.Vec2, trans *domain.Transform, vel *domain.Velocity, ai *domain.AIState, shipCfg *domain.ShipConfig) {
	var myFactionID uint32 = 0
	if fVal, ok := world.GetComponent(id, domain.FactionMember{}); ok {
		myFactionID = fVal.(*domain.FactionMember).FactionID
	}

	if ai.TargetID == 0 {
		stationID, _, found := s.findNearestFactionStation(world, myPos, myFactionID)
		if found {
			ai.TargetID = stationID
		}
	}

	defCenter := mathutil.NewVec2(ai.HomePos.X, ai.HomePos.Y)
	if ai.TargetID != 0 {
		if targetPos, exists := s.getEntityPos(world, ai.TargetID); exists {
			defCenter = targetPos
		} else {
			ai.TargetID = 0
		}
	}

	dist := myPos.Distance(defCenter)
	if dist > 800.0 {
		dir := defCenter.Sub(myPos).Normalize()
		vel.X = dir.X * shipCfg.MaxSpeed * 0.7
		vel.Y = dir.Y * shipCfg.MaxSpeed * 0.7
	} else {
		if ai.StateTimer <= 0 {
			ai.StateTimer = 3.0 + s.random.Float64()*3.0
			vel.X = (s.random.Float32() - 0.5) * shipCfg.MaxSpeed * 0.4
			vel.Y = (s.random.Float32() - 0.4) * shipCfg.MaxSpeed * 0.4
		}
	}
}

func (s *AISystem) findNearestFactionStation(world *ecs.World, myPos mathutil.Vec2, factionID uint32) (domain.EntityID, mathutil.Vec2, bool) {
	mask := ecs.BuildMask(domain.Transform{}, domain.StationOwnership{})
	entities := world.Query(mask)

	var nearestID domain.EntityID
	var nearestPos mathutil.Vec2
	minDist := float32(math.MaxFloat32)
	found := false

	for _, id := range entities {
		tVal, _ := world.GetComponent(id, domain.Transform{})
		oVal, _ := world.GetComponent(id, domain.StationOwnership{})

		trans := tVal.(*domain.Transform)
		ownership := oVal.(*domain.StationOwnership)

		if ownership.CorpID != factionID {
			continue
		}

		pos := mathutil.NewVec2(trans.X, trans.Y)
		dist := myPos.Distance(pos)
		if dist < minDist {
			minDist = dist
			nearestID = id
			nearestPos = pos
			found = true
		}
	}
	return nearestID, nearestPos, found
}
