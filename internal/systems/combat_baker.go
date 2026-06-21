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

		// Phase 1: bake combat stats from the fitting catalog (hull + weapons + flux + armor).
		// Max stats come from the hull loadout; current HP/Shield carry over from the persisted
		// FleetShip so battle damage persists across engagements.
		cfg := domain.DefaultLoadoutForShipType(ship.ShipType)
		stats := domain.ComputeStats(cfg)

		hp := ship.Health
		if hp > int32(stats.MaxHP) {
			hp = int32(stats.MaxHP)
		}
		targetWorld.AddComponent(shipID, &domain.Health{Current: hp, Max: int32(stats.MaxHP)})

		if stats.ShieldType != "none" {
			shieldCur := ship.Shield
			if shieldCur > int32(stats.MaxShield) {
				shieldCur = int32(stats.MaxShield)
			}
			targetWorld.AddComponent(shipID, &domain.Shield{
				Current:    shieldCur,
				Max:        int32(stats.MaxShield),
				RegenRate:  stats.MaxShield * 0.05,
				Type:       stats.ShieldType,
				Arc:        stats.ShieldArc,
				Efficiency: stats.ShieldEfficiency,
			})
		}

		targetWorld.AddComponent(shipID, &domain.ArmorGrid{Current: stats.MaxArmor, Max: stats.MaxArmor})

		targetWorld.AddComponent(shipID, &domain.ShipConfig{
			ShipType: ship.ShipType,
			MaxSpeed: stats.MaxSpeed,
			TurnRate: stats.TurnRate,
		})

		targetWorld.AddComponent(shipID, &domain.FluxState{
			Current:         0,
			Capacity:        stats.MaxFlux,
			DissipationRate: stats.FluxDissipation,
		})

		// Per-mount weapons — the actual damage source in combat.
		weapons := make([]domain.FittedWeaponState, len(stats.Weapons))
		copy(weapons, stats.Weapons)
		targetWorld.AddComponent(shipID, &domain.WeaponGroup{Weapons: weapons})

		// Transient combat FX (shots fired / last damage type) for the snapshot.
		targetWorld.AddComponent(shipID, &domain.CombatFx{})

		// The Weapon component is the AI's targeting controller: AISystem sets Active/TargetID and
		// uses Range for standoff; CombatSystem reads the target but deals damage via WeaponGroup.
		ctrlRange, ctrlCooldown := primaryWeaponProfile(stats.Weapons)
		targetWorld.AddComponent(shipID, &domain.Weapon{
			Type:     domain.WeaponLaser,
			Damage:   0, // damage now comes from WeaponGroup mounts
			Range:    ctrlRange,
			Cooldown: ctrlCooldown,
			Active:   false,
		})

		// Компонент команды боя
		targetWorld.AddComponent(shipID, &domain.CombatTeam{
			TeamID:  teamID,
			FleetID: fleetEntityID,
		})

		// Тактика боя (Phase 1.5): роль и стратегия из FleetShip (или дефолты по индексу).
		role, stance := domain.ResolveTactics(ship.Role, ship.Strategy, i)
		targetWorld.AddComponent(shipID, &domain.CombatRole{Role: role})
		targetWorld.AddComponent(shipID, &domain.CombatStrategy{Stance: stance})

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
// Флот считается уничтоженным только если не уцелел ни один корабль. Если флагман погиб,
// но эскорт выжил, флот продолжает существовать, а ведущим становится первый уцелевший корабль.
func PackFleet(sourceWorld, targetWorld *ecs.World, fleetEntityID domain.EntityID, shipEntities []domain.EntityID) bool {
	fleetVal, ok := sourceWorld.GetComponent(fleetEntityID, domain.Fleet{})
	if !ok {
		return false
	}
	fleet := fleetVal.(*domain.Fleet)

	if len(shipEntities) == 0 {
		fleet.Ships = nil
		return false
	}

	var survivingShips []domain.FleetShip
	// Синхронизируем HP/SH ведущего (первого выжившего) корабля на сущность флота в основном
	// мире только один раз — иначе HUD/снимок показывают доболевые значения после боя.
	entityStatsSynced := false

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

		// Синхронизируем HP/SH ведущего (первого выжившего) корабля на сущность флота в основном
		// мире. Флагман может быть уничтожен, но флот выживает за счёт эскорта — тогда ведущим
		// становится первый уцелевший корабль.
		if !entityStatsSynced {
			entityStatsSynced = true
			if hMainVal, ok := sourceWorld.GetComponent(fleetEntityID, domain.Health{}); ok {
				hMain := hMainVal.(*domain.Health)
				hMain.Current = h.Current
				hMain.Max = h.Max
			} else {
				sourceWorld.AddComponent(fleetEntityID, &domain.Health{Current: h.Current, Max: h.Max})
			}
			if sMainVal, ok := sourceWorld.GetComponent(fleetEntityID, domain.Shield{}); ok {
				sMain := sMainVal.(*domain.Shield)
				sMain.Current = s.Current
				sMain.Max = s.Max
			} else {
				sourceWorld.AddComponent(fleetEntityID, &domain.Shield{Current: s.Current, Max: s.Max, RegenRate: 1.0})
			}
		}

		// Cargo и PlayerData (Credits) переносятся обратно только с выжившего флагмана,
		// так как именно ему они копируются при распаковке (UnpackFleet).
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

// primaryWeaponProfile derives the AI standoff range and a representative cooldown from a
// ship's weapon mounts. AISystem keeps a ship at ~70-90% of this range, so we use the longest
// mount range (the ship closes to where its furthest gun can hit). Falls back to a sane default
// for weaponless ships.
func primaryWeaponProfile(weapons []domain.FittedWeaponState) (rng float32, cooldown float32) {
	rng = 300
	cooldown = 1.0
	maxRange := float32(0)
	for i := range weapons {
		if weapons[i].Definition.Range > maxRange {
			maxRange = weapons[i].Definition.Range
			cooldown = weapons[i].Definition.Cooldown
		}
	}
	if maxRange > 0 {
		rng = maxRange
	}
	return rng, cooldown
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
