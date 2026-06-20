package systems

import (
	"math"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// UnpackFleet распаковывает флот из sourceWorld в targetWorld.
// Первый корабль становится Флагманом с ID, равным fleetEntityID.
// Последующие корабли становятся эскортом с динамическими ID и ИИ.
func UnpackFleet(sourceWorld, targetWorld *ecs.World, fleetEntityID domain.EntityID, teamID uint32, baseX, baseY float32, angleOffset float64) []domain.EntityID {
	fleetVal, ok := sourceWorld.GetComponent(fleetEntityID, domain.Fleet{})
	if !ok {
		return nil
	}
	fleet := fleetVal.(*domain.Fleet)
	if len(fleet.Ships) == 0 {
		return nil
	}

	// Определяем тип сущности (игрок или NPC)
	eType, exists := sourceWorld.GetEntityType(fleetEntityID)
	if !exists {
		eType = domain.EntityNPC
	}

	// Проверяем принадлежность к корпорации
	var corpID uint32
	var hasCorp bool
	if corpVal, ok := sourceWorld.GetComponent(fleetEntityID, domain.CorporationMember{}); ok {
		corpID = corpVal.(*domain.CorporationMember).CorpID
		hasCorp = true
	}

	var spawnedIDs []domain.EntityID

	for i, ship := range fleet.Ships {
		var shipID domain.EntityID
		var isFlagship bool

		if i == 0 {
			// Флагман получает оригинальный ID игрока/NPC для сохранения маршрутизации
			shipID = fleetEntityID
			targetWorld.RegisterEntityWithID(shipID, eType)
			isFlagship = true
		} else {
			// Эскорт получает новый динамический NPC ID
			shipID = targetWorld.CreateEntity(domain.EntityNPC)
			isFlagship = false
		}

		// Вычисляем координаты спавна с отступом для эскорта
		x := baseX
		y := baseY
		if !isFlagship {
			// Размещаем эскорт полукругом позади флагмана
			offsetAngle := angleOffset + math.Pi + (float64(i)*0.5 - 0.75)
			x += float32(math.Cos(offsetAngle) * 80.0)
			y += float32(math.Sin(offsetAngle) * 80.0)
		}

		// Базовые компоненты движения
		targetWorld.AddComponent(shipID, &domain.Transform{
			X:        x,
			Y:        y,
			Rotation: float32(angleOffset),
		})
		targetWorld.AddComponent(shipID, &domain.Velocity{X: 0, Y: 0})

		// Компоненты здоровья и щитов из структуры корабля
		targetWorld.AddComponent(shipID, &domain.Health{
			Current: ship.Health,
			Max:     ship.MaxHealth,
		})
		targetWorld.AddComponent(shipID, &domain.Shield{
			Current:   ship.Shield,
			Max:       ship.MaxShield,
			RegenRate: 1.0,
		})

		// Компоненты характеристики корабля и оружия
		speed, turn, dmg, rng, cooldown := getShipStats(ship.ShipType)
		targetWorld.AddComponent(shipID, &domain.ShipConfig{
			ShipType: ship.ShipType,
			MaxSpeed: speed,
			TurnRate: turn,
		})
		targetWorld.AddComponent(shipID, &domain.Weapon{
			Type:     domain.WeaponLaser,
			Damage:   dmg,
			Range:    rng,
			Cooldown: cooldown,
			Active:   false,
		})

		// Компонент команды боя
		targetWorld.AddComponent(shipID, &domain.CombatTeam{
			TeamID:  teamID,
			FleetID: fleetEntityID,
		})

		// Копируем принадлежность к корпорации
		if hasCorp {
			targetWorld.AddComponent(shipID, &domain.CorporationMember{
				CorpID: corpID,
				Role:   "Member",
			})
		}

		// Все корабли в боевом инстансе (флагманы и эскорт) получают AIState с поведением Attack для полностью автоматического боя
		targetWorld.AddComponent(shipID, &domain.AIState{
			Behavior:   domain.BehaviorAttack,
			HomePos:    domain.Transform{X: x, Y: y},
			StateTimer: 0,
		})

		if isFlagship {
			// Если флагман принадлежит игроку, скопируем PlayerData для отображения имени/кредитов
			if pDataVal, hasPData := sourceWorld.GetComponent(fleetEntityID, domain.PlayerData{}); hasPData {
				pData := pDataVal.(*domain.PlayerData)
				targetWorld.AddComponent(shipID, &domain.PlayerData{
					AccountID: pData.AccountID,
					Name:      pData.Name,
					Credits:   pData.Credits,
					SystemID:  pData.SystemID,
				})
			}
			// Скопируем также Cargo флагману (для отображения или подбора лута)
			if cargoVal, hasCargo := sourceWorld.GetComponent(fleetEntityID, domain.Cargo{}); hasCargo {
				cargo := cargoVal.(*domain.Cargo)
				targetWorld.AddComponent(shipID, &domain.Cargo{
					Items:    append([]domain.ItemInstance{}, cargo.Items...),
					Capacity: cargo.Capacity,
				})
			}
		}

		spawnedIDs = append(spawnedIDs, shipID)
	}

	return spawnedIDs
}

// PackFleet собирает выжившие корабли из targetWorld обратно во флот в sourceWorld.
// Если флагман уничтожен, весь флот считается уничтоженным.
func PackFleet(sourceWorld, targetWorld *ecs.World, fleetEntityID domain.EntityID, shipEntities []domain.EntityID) bool {
	fleetVal, ok := sourceWorld.GetComponent(fleetEntityID, domain.Fleet{})
	if !ok {
		return false
	}
	fleet := fleetVal.(*domain.Fleet)

	// Флагман - это первый элемент в переданном слайсе shipEntities (по определению UnpackFleet)
	if len(shipEntities) == 0 {
		fleet.Ships = nil
		return false
	}



	var survivingShips []domain.FleetShip

	// Синхронизируем состояние выживших кораблей
	for i, shipID := range shipEntities {
		_, alive := targetWorld.GetEntityType(shipID)
		if !alive {
			continue // Корабль уничтожен в бою
		}

		hVal, hOk := targetWorld.GetComponent(shipID, domain.Health{})
		sVal, sOk := targetWorld.GetComponent(shipID, domain.Shield{})
		cfgVal, cfgOk := targetWorld.GetComponent(shipID, domain.ShipConfig{})

		if !hOk || !sOk || !cfgOk {
			continue
		}

		h := hVal.(*domain.Health)
		s := sVal.(*domain.Shield)
		cfg := cfgVal.(*domain.ShipConfig)

		// Находим исходный корабль по индексу, чтобы сохранить его ID и вместимость
		var originalShip domain.FleetShip
		if i < len(fleet.Ships) {
			originalShip = fleet.Ships[i]
		} else {
			originalShip = domain.FleetShip{
				ShipID:        uint32(i + 1),
				ShipType:      cfg.ShipType,
				CargoCapacity: 100,
			}
		}

		survivingShips = append(survivingShips, domain.FleetShip{
			ShipID:        originalShip.ShipID,
			ShipType:      cfg.ShipType,
			Health:        h.Current,
			MaxHealth:     h.Max,
			Shield:        s.Current,
			MaxShield:     s.Max,
			CargoCapacity: originalShip.CargoCapacity,
		})

		// Если это флагман, синхронизируем Cargo и PlayerData (Credits) обратно
		if i == 0 {
			if cargoVal, hasCargo := targetWorld.GetComponent(shipID, domain.Cargo{}); hasCargo {
				cSourceVal, hasSourceCargo := sourceWorld.GetComponent(fleetEntityID, domain.Cargo{})
				if hasSourceCargo {
					cSource := cSourceVal.(*domain.Cargo)
					cSource.Items = append([]domain.ItemInstance{}, cargoVal.(*domain.Cargo).Items...)
				}
			}
			if pDataVal, hasPData := targetWorld.GetComponent(shipID, domain.PlayerData{}); hasPData {
				pSourceVal, hasSourcePData := sourceWorld.GetComponent(fleetEntityID, domain.PlayerData{})
				if hasSourcePData {
					pSource := pSourceVal.(*domain.PlayerData)
					pSource.Credits = pDataVal.(*domain.PlayerData).Credits
				}
			}
		}

		// Удаляем сущность из боевого инстанса
		targetWorld.DestroyEntity(shipID)
	}

	fleet.Ships = survivingShips
	return len(survivingShips) > 0
}

func getShipStats(shipType string) (speed, turn float32, dmg int32, rng float32, cooldown float32) {
	switch shipType {
	case "fighter":
		return 120, 1.8, 10, 500, 0.5
	case "patrol":
		return 100, 1.5, 8, 400, 0.8
	case "pirate":
		return 90, 1.4, 7, 350, 0.9
	case "miner":
		return 60, 1.0, 5, 300, 1.0
	case "cargo_helper", "cargo":
		return 50, 0.8, 3, 250, 1.5
	default:
		return 80, 1.2, 6, 300, 1.0
	}
}
