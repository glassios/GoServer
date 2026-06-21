package systems

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

type JumpGateSystem struct {
	bus             messaging.MessageBus
	currentSystemID uint32
	logger          *zap.Logger
}

func NewJumpGateSystem(bus messaging.MessageBus, currentSystemID uint32, logger *zap.Logger) *JumpGateSystem {
	return &JumpGateSystem{
		bus:             bus,
		currentSystemID: currentSystemID,
		logger:          logger,
	}
}

func (s *JumpGateSystem) Name() string {
	return "JumpGateSystem"
}

func (s *JumpGateSystem) Priority() int {
	return 90 // Runs after movement to check final positions
}

func (s *JumpGateSystem) Update(world *ecs.World, dt float64) {
	// 1. Get all Jump Gates
	gateMask := ecs.BuildMask(domain.Transform{}, domain.JumpGate{})
	gates := world.Query(gateMask)
	if len(gates) == 0 {
		return
	}

	// 2. Get all movable entities (Players or NPCs)
	transformMask := ecs.BuildMask(domain.Transform{})
	candidates := world.Query(transformMask)

	var players []domain.EntityID
	for _, id := range candidates {
		_, isPlayer := world.GetComponent(id, domain.PlayerData{})
		_, isNPC := world.GetComponent(id, domain.AIState{})
		if isPlayer || isNPC {
			players = append(players, id)
		}
	}
	if len(players) == 0 {
		return
	}

	for _, gateID := range gates {
		gateTVal, _ := world.GetComponent(gateID, domain.Transform{})
		gateT := gateTVal.(*domain.Transform)
		gateVal, _ := world.GetComponent(gateID, domain.JumpGate{})
		gate := gateVal.(*domain.JumpGate)

		for _, playerID := range players {
			playerTVal, ok := world.GetComponent(playerID, domain.Transform{})
			if !ok {
				continue
			}
			playerT := playerTVal.(*domain.Transform)

			// Calculate distance squared
			dx := playerT.X - gateT.X
			dy := playerT.Y - gateT.Y
			dist2 := dx*dx + dy*dy

			// Jump threshold: distance < 100 units (dist2 < 10000)
			if dist2 < 10000 {
				s.logger.Info("Player near jump gate. Triggering cross-node migration...",
					zap.Uint64("playerID", uint64(playerID)),
					zap.Uint32("targetSystemID", gate.TargetSystemID),
				)

				// Perform migration serialization
				payload := SerializePlayer(world, playerID)

				// Set spawn coordinates on target system
				payload.X = gate.TargetX
				payload.Y = gate.TargetY
				if !payload.IsNpc {
					payload.Vx = 0
					payload.Vy = 0
				}

				req := &protocol.SystemTransferRequest{
					PlayerId:       uint64(playerID),
					TargetSystemId: gate.TargetSystemID,
					SpawnX:         gate.TargetX,
					SpawnY:         gate.TargetY,
					Payload:        payload,
				}

				reqData, err := proto.Marshal(req)
				if err != nil {
					s.logger.Error("Failed to marshal system transfer request", zap.Error(err))
					continue
				}

				// Call synchronous NATS Request
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				resData, err := s.bus.Request(ctx, fmt.Sprintf("system.%d.transfer", gate.TargetSystemID), reqData)
				cancel()

				if err != nil {
					s.logger.Error("Migration request failed or timed out", zap.Error(err))
					continue // Don't delete player if transfer failed!
				}

				var resp protocol.SystemTransferResponse
				if err := proto.Unmarshal(resData, &resp); err != nil {
					s.logger.Error("Failed to unmarshal transfer response", zap.Error(err))
					continue
				}

				if !resp.Success {
					s.logger.Error("Target node rejected transfer", zap.String("error", resp.ErrorMessage))
					continue
				}

				// Success! Delete player from local system node
				world.DestroyEntity(playerID)

				// Notify gateway routing table to update routing for this player
				routingUpdate := fmt.Sprintf("%d,%d", playerID, gate.TargetSystemID)
				_ = s.bus.Publish("system.routing.update", []byte(routingUpdate))

				s.logger.Info("Migration completed successfully. Entity removed from current system.", zap.Uint64("playerID", uint64(playerID)))
			}
		}
	}
}

// SerializePlayer extracts all player or NPC components into a Protobuf message.
func SerializePlayer(world *ecs.World, id domain.EntityID) *protocol.PlayerMigrationPayload {
	tVal, foundT := world.GetComponent(id, domain.Transform{})
	vVal, foundV := world.GetComponent(id, domain.Velocity{})
	hVal, foundH := world.GetComponent(id, domain.Health{})
	sVal, foundS := world.GetComponent(id, domain.Shield{})
	wVal, foundW := world.GetComponent(id, domain.Weapon{})
	cVal, foundC := world.GetComponent(id, domain.Cargo{})
	lVal, foundL := world.GetComponent(id, domain.MiningLaser{})
	cfgVal, foundCfg := world.GetComponent(id, domain.ShipConfig{})
	pVal, foundP := world.GetComponent(id, domain.PlayerData{})
	fVal, foundF := world.GetComponent(id, domain.FactionMember{})

	payload := &protocol.PlayerMigrationPayload{
		PlayerId: uint64(id),
	}

	if foundT {
		t := tVal.(*domain.Transform)
		payload.X = t.X
		payload.Y = t.Y
		payload.Rotation = t.Rotation
	}
	if foundV {
		v := vVal.(*domain.Velocity)
		payload.Vx = v.X
		payload.Vy = v.Y
	}
	if foundH {
		h := hVal.(*domain.Health)
		payload.Hp = h.Current
		payload.MaxHp = h.Max
	}
	if foundS {
		s := sVal.(*domain.Shield)
		payload.Shield = s.Current
		payload.MaxShield = s.Max
		payload.ShieldRegenRate = s.RegenRate
	}
	if foundW {
		w := wVal.(*domain.Weapon)
		payload.WeaponType = string(w.Type)
		payload.WeaponDamage = w.Damage
		payload.WeaponRange = w.Range
		payload.WeaponCooldown = w.Cooldown
	}
	if foundC {
		c := cVal.(*domain.Cargo)
		payload.CargoCapacity = c.Capacity
		payload.CargoItems = make([]*protocol.ItemInstanceProto, 0, len(c.Items))
		for _, item := range c.Items {
			payload.CargoItems = append(payload.CargoItems, &protocol.ItemInstanceProto{
				Id:           item.ID,
				DefinitionId: item.DefinitionID,
				Quantity:     item.Quantity,
				LocationType: item.LocationType,
				LocationId:   item.LocationID,
				OwnerId:      item.OwnerID,
				State:        item.State,
				Name:         string(domain.IDToResource[item.DefinitionID]),
			})
		}
	}
	if foundL {
		l := lVal.(*domain.MiningLaser)
		payload.MiningPower = l.Power
		payload.MiningRange = l.Range
	}
	if foundCfg {
		cfg := cfgVal.(*domain.ShipConfig)
		payload.ShipType = cfg.ShipType
		payload.MaxSpeed = cfg.MaxSpeed
		payload.TurnRate = cfg.TurnRate
	}
	if foundP {
		p := pVal.(*domain.PlayerData)
		payload.AccountId = p.AccountID
		payload.PlayerName = p.Name
		payload.Credits = p.Credits
		payload.SessionId = p.SessionID
	}
	if foundF {
		f := fVal.(*domain.FactionMember)
		payload.FactionId = f.FactionID
	}

	// Сериализуем весь флот (по-корабельно), чтобы HP/SH/состав сохранялись при прыжке через гейт.
	if flVal, foundFleet := world.GetComponent(id, domain.Fleet{}); foundFleet {
		fleet := flVal.(*domain.Fleet)
		payload.FleetShips = make([]*protocol.FleetShipProto, 0, len(fleet.Ships))
		for _, ship := range fleet.Ships {
			payload.FleetShips = append(payload.FleetShips, &protocol.FleetShipProto{
				ShipId:         ship.ShipID,
				ShipType:       ship.ShipType,
				Health:         ship.Health,
				MaxHealth:      ship.MaxHealth,
				Shield:         ship.Shield,
				MaxShield:      ship.MaxShield,
				CargoCapacity:  ship.CargoCapacity,
				Role:           ship.Role,
				Strategy:       ship.Strategy,
				Customized:     ship.Customized,
				HullId:         ship.HullID,
				FittedWeapons:  ship.FittedWeapons,
				FittedHullmods: ship.FittedHullmods,
				Vents:          ship.Vents,
				Capacitors:     ship.Capacitors,
			})
		}
	}

	// Carry skill progression (Phase 3) so leveling survives a cross-node jump.
	if pgVal, foundPg := world.GetComponent(id, domain.PlayerProgress{}); foundPg {
		pg := pgVal.(*domain.PlayerProgress)
		for _, k := range domain.SkillKeys {
			st := pg.Skills[k]
			if st == nil {
				continue
			}
			payload.Skills = append(payload.Skills, &protocol.SkillProto{
				Key:    k,
				Level:  st.Level,
				Xp:     st.XP,
				XpNext: domain.XPForNextLevel(st.Level),
			})
		}
	}

	eType, hasEType := world.GetEntityType(id)
	if hasEType && eType == domain.EntityNPC {
		payload.IsNpc = true
		if aiVal, foundAI := world.GetComponent(id, domain.AIState{}); foundAI {
			ai := aiVal.(*domain.AIState)
			payload.AiBehavior = string(ai.Behavior)
			payload.AiTargetId = uint64(ai.TargetID)
		}
	}

	return payload
}

// DeserializePlayer reconstructs all player components from a Protobuf message.
func DeserializePlayer(world *ecs.World, payload *protocol.PlayerMigrationPayload) domain.EntityID {
	playerID := domain.EntityID(payload.PlayerId)
	if payload.IsNpc {
		world.RegisterEntityWithID(playerID, domain.EntityNPC)
		world.AddComponent(playerID, &domain.AIState{
			Behavior: domain.AIBehavior(payload.AiBehavior),
			TargetID: domain.EntityID(payload.AiTargetId),
		})
	} else {
		world.RegisterEntityWithID(playerID, domain.EntityPlayer)
	}

	world.AddComponent(playerID, &domain.Transform{X: payload.X, Y: payload.Y, Rotation: payload.Rotation})
	world.AddComponent(playerID, &domain.Velocity{X: payload.Vx, Y: payload.Vy})
	world.AddComponent(playerID, &domain.Health{Current: payload.Hp, Max: payload.MaxHp})
	world.AddComponent(playerID, &domain.Shield{Current: payload.Shield, Max: payload.MaxShield, RegenRate: payload.ShieldRegenRate})

	world.AddComponent(playerID, &domain.ShipConfig{
		ShipType: payload.ShipType,
		MaxSpeed: payload.MaxSpeed,
		TurnRate: payload.TurnRate,
	})

	world.AddComponent(playerID, &domain.Visibility{
		Radius:          1500.0,
		VisibleEntities: make(map[domain.EntityID]struct{}),
	})

	world.AddComponent(playerID, &domain.PlayerData{
		AccountID: payload.AccountId,
		Name:      payload.PlayerName,
		Credits:   payload.Credits,
		SessionID: payload.SessionId,
	})

	world.AddComponent(playerID, &domain.FactionMember{
		FactionID: payload.FactionId,
	})

	// Reconstruct fleet component
	shipType := payload.ShipType
	if shipType == "" {
		shipType = "fighter"
	}

	var ships []domain.FleetShip
	if len(payload.FleetShips) > 0 {
		// Восстанавливаем точный состав флота с сохранением HP/SH каждого корабля.
		for _, fs := range payload.FleetShips {
			ships = append(ships, domain.FleetShip{
				ShipID:         fs.ShipId,
				ShipType:       fs.ShipType,
				Health:         fs.Health,
				MaxHealth:      fs.MaxHealth,
				Shield:         fs.Shield,
				MaxShield:      fs.MaxShield,
				CargoCapacity:  fs.CargoCapacity,
				Role:           fs.Role,
				Strategy:       fs.Strategy,
				Customized:     fs.Customized,
				HullID:         fs.HullId,
				FittedWeapons:  fs.FittedWeapons,
				FittedHullmods: fs.FittedHullmods,
				Vents:          fs.Vents,
				Capacitors:     fs.Capacitors,
			})
		}
	} else {
		// Fallback: старый формат миграции/новый спавн без данных о флоте.
		ships = []domain.FleetShip{
			{
				ShipID:        1,
				ShipType:      shipType,
				Health:        payload.Hp,
				MaxHealth:     payload.MaxHp,
				Shield:        payload.Shield,
				MaxShield:     payload.MaxShield,
				CargoCapacity: payload.CargoCapacity,
			},
		}
		// For players, also recreate the default secondary escort miner ship
		if !payload.IsNpc {
			ships = append(ships, domain.FleetShip{
				ShipID:        2,
				ShipType:      "miner",
				Health:        80,
				MaxHealth:     80,
				Shield:        30,
				MaxShield:     30,
				CargoCapacity: 150,
			})
		}
	}
	world.AddComponent(playerID, &domain.Fleet{Ships: ships})

	var cargoItems []domain.ItemInstance
	for _, item := range payload.CargoItems {
		cargoItems = append(cargoItems, domain.ItemInstance{
			ID:           item.Id,
			DefinitionID: item.DefinitionId,
			Quantity:     item.Quantity,
			LocationType: item.LocationType,
			LocationID:   item.LocationId,
			OwnerID:      item.OwnerId,
			State:        item.State,
		})
	}
	world.AddComponent(playerID, &domain.Cargo{
		Items:    cargoItems,
		Capacity: payload.CargoCapacity,
	})

	world.AddComponent(playerID, &domain.Weapon{
		Type:     domain.WeaponType(payload.WeaponType),
		Damage:   payload.WeaponDamage,
		Range:    payload.WeaponRange,
		Cooldown: payload.WeaponCooldown,
	})

	world.AddComponent(playerID, &domain.MiningLaser{
		Power: payload.MiningPower,
		Range: payload.MiningRange,
	})

	// Restore skill progression (Phase 3).
	progress := domain.NewPlayerProgress()
	for _, sk := range payload.Skills {
		lvl := sk.Level
		if lvl < 1 {
			lvl = 1
		}
		progress.Skills[sk.Key] = &domain.SkillState{Level: lvl, XP: sk.Xp}
	}
	world.AddComponent(playerID, progress)

	return playerID
}
