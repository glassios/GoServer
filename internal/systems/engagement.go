package systems

import (
	"strings"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

type FleetEngagementSystem struct {
	im   *InstanceManager
	grid *spatial.HashGrid
}

func NewFleetEngagementSystem(im *InstanceManager, grid *spatial.HashGrid) *FleetEngagementSystem {
	return &FleetEngagementSystem{
		im:   im,
		grid: grid,
	}
}

func (s *FleetEngagementSystem) Name() string {
	return "FleetEngagementSystem"
}

func (s *FleetEngagementSystem) Priority() int {
	return 90 // Выполняется после движения флотов, чтобы проверить новые координаты
}

func (s *FleetEngagementSystem) Update(world *ecs.World, dt float64) {
	// Радиус сближения для присоединения к бою (30 единиц)
	engageDistSq := float32(30.0 * 30.0)

	// 1. Ищем все флоты на карте, которые еще не в бою
	mask := ecs.BuildMask(domain.Transform{}, domain.Fleet{})
	entities := world.Query(mask)

	var freeFleets []domain.EntityID
	for _, id := range entities {
		if _, ok := world.GetComponent(id, domain.CombatState{}); !ok {
			freeFleets = append(freeFleets, id)
		}
	}

	// 2. Детектируем новые столкновения свободных флотов
	if len(freeFleets) >= 2 {
		for i := 0; i < len(freeFleets); i++ {
			id1 := freeFleets[i]
			t1Val, _ := world.GetComponent(id1, domain.Transform{})
			t1 := t1Val.(*domain.Transform)

			for j := i + 1; j < len(freeFleets); j++ {
				id2 := freeFleets[j]
				t2Val, _ := world.GetComponent(id2, domain.Transform{})
				t2 := t2Val.(*domain.Transform)

				dx := t1.X - t2.X
				dy := t1.Y - t2.Y
				distSq := dx*dx + dy*dy

				// Вычисляем радиус авто-боя по дальности оружия флотов (по умолчанию 30)
				var range1 float32 = 30.0
				if wVal1, ok := world.GetComponent(id1, domain.Weapon{}); ok {
					range1 = wVal1.(*domain.Weapon).Range
				}
				var range2 float32 = 30.0
				if wVal2, ok := world.GetComponent(id2, domain.Weapon{}); ok {
					range2 = wVal2.(*domain.Weapon).Range
				}

				if AreHostile(world, id1, id2) {
					// Бой начинается только если агрессор находится в зоне атаки от цели
					agg1 := isAggressor(world, id1, id2)
					agg2 := isAggressor(world, id2, id1)

					shouldEngage := false
					if agg1 && distSq < range1*range1 {
						shouldEngage = true
					}
					if agg2 && distSq < range2*range2 {
						shouldEngage = true
					}

					if shouldEngage {
						// Если один из них майнер, помечаем второго как MinerAttacker
						if IsMiner(world, id2) {
							world.AddComponent(id1, &domain.MinerAttacker{IsCriminal: true})
						} else if IsMiner(world, id1) {
							world.AddComponent(id2, &domain.MinerAttacker{IsCriminal: true})
						}

						// Создаем боевой инстанс
						instID := s.im.CreateCombatInstance(world, id1, id2)
						if instID == 0 {
							continue
						}

						// Создаем маркер боя (Combat Marker) на месте столкновения
						midX := (t1.X + t2.X) / 2
						midY := (t1.Y + t2.Y) / 2

						marker := world.CreateEntity(domain.EntityCombatMarker)
						world.AddComponent(marker, &domain.Transform{X: midX, Y: midY})
						world.AddComponent(marker, &domain.CombatMarker{
							CombatInstanceID: instID,
							AttackerID:       id1,
							DefenderID:       id2,
						})
						world.AddComponent(marker, &domain.Health{Current: 100, Max: 100})
						world.AddComponent(marker, &domain.ShipConfig{ShipType: "combat_marker"})
						world.AddComponent(marker, &domain.PlayerData{Name: "Active Battle", Credits: 0})

						s.grid.Insert(marker, midX, midY)
						break
					}
				}
			}
		}
	}

	// 3. Проверка приближения свободных NPC к маркерам активного боя для вмешательства
	markers := world.Query(ecs.BuildMask(domain.Transform{}, domain.CombatMarker{}))
	for _, markerID := range markers {
		cmVal, _ := world.GetComponent(markerID, domain.CombatMarker{})
		cm := cmVal.(*domain.CombatMarker)

		tMarkerVal, _ := world.GetComponent(markerID, domain.Transform{})
		tMarker := tMarkerVal.(*domain.Transform)

		for _, freeID := range freeFleets {
			// Проверяем, является ли свободный флот NPC
			eType, exists := world.GetEntityType(freeID)
			if !exists || eType != domain.EntityNPC {
				continue // Игроки присоединяются вручную
			}

			tVal, _ := world.GetComponent(freeID, domain.Transform{})
			t := tVal.(*domain.Transform)

			dx := t.X - tMarker.X
			dy := t.Y - tMarker.Y
			distSq := dx*dx + dy*dy

			if distSq < engageDistSq {
				if IsPatrol(world, freeID) {
					// Патруль помогает защитникам (DefenderID)
					_ = s.im.JoinCombatInstance(world, cm.CombatInstanceID, freeID, cm.DefenderID)
				} else if IsPirate(world, freeID) {
					// Пират помогает атакующим (AttackerID)
					_ = s.im.JoinCombatInstance(world, cm.CombatInstanceID, freeID, cm.AttackerID)
				}
			}
		}
	}
}

func IsMiner(world *ecs.World, id domain.EntityID) bool {
	if pDataVal, ok := world.GetComponent(id, domain.PlayerData{}); ok {
		pData := pDataVal.(*domain.PlayerData)
		return strings.Contains(strings.ToLower(pData.Name), "miner")
	}
	return false
}

func IsPatrol(world *ecs.World, id domain.EntityID) bool {
	if pDataVal, ok := world.GetComponent(id, domain.PlayerData{}); ok {
		pData := pDataVal.(*domain.PlayerData)
		if strings.Contains(strings.ToLower(pData.Name), "patrol") || strings.Contains(strings.ToLower(pData.Name), "enforcer") {
			return true
		}
	}
	if cfgVal, ok := world.GetComponent(id, domain.ShipConfig{}); ok {
		cfg := cfgVal.(*domain.ShipConfig)
		return cfg.ShipType == "patrol"
	}
	return false
}

func IsPirate(world *ecs.World, id domain.EntityID) bool {
	fVal, hasFaction := world.GetComponent(id, domain.FactionMember{})
	if hasFaction {
		fID := fVal.(*domain.FactionMember).FactionID
		if fID == 1 {
			eType, exists := world.GetEntityType(id)
			return exists && eType == domain.EntityNPC
		}
	}
	return false
}

func AreHostile(world *ecs.World, id1, id2 domain.EntityID) bool {
	if id1 == id2 {
		return false
	}

	isP1 := IsPirate(world, id1)
	isP2 := IsPirate(world, id2)
	isM1 := IsMiner(world, id1)
	isM2 := IsMiner(world, id2)
	isPat1 := IsPatrol(world, id1)
	isPat2 := IsPatrol(world, id2)

	eType1, exists1 := world.GetEntityType(id1)
	eType2, exists2 := world.GetEntityType(id2)
	isPlayer1 := exists1 && eType1 == domain.EntityPlayer
	isPlayer2 := exists2 && eType2 == domain.EntityPlayer

	_, isCrim1 := world.GetComponent(id1, domain.MinerAttacker{})
	_, isCrim2 := world.GetComponent(id2, domain.MinerAttacker{})

	// НОВОЕ: Проверка активной атаки игрока (ручное нападение через "Attack Fleet")
	if isPlayer1 {
		if wVal, ok := world.GetComponent(id1, domain.Weapon{}); ok {
			w := wVal.(*domain.Weapon)
			if w.Active && w.TargetID == id2 {
				return true
			}
		}
	}
	if isPlayer2 {
		if wVal, ok := world.GetComponent(id2, domain.Weapon{}); ok {
			w := wVal.(*domain.Weapon)
			if w.Active && w.TargetID == id1 {
				return true
			}
		}
	}

	// 1. Пираты враждебны ко всем, кроме других пиратов NPC
	if isP1 {
		return !isP2
	}
	if isP2 {
		return !isP1
	}

	// 2. Патрули атакуют только пиратов и тех, кто нападал на шахтеров (MinerAttacker)
	if isPat1 {
		return isP2 || isCrim2
	}
	if isPat2 {
		return isP1 || isCrim1
	}

	// 3. Игроки по умолчанию мирные, кроме случаев ручной атаки (проверенной выше) или если игрок преступник
	if isPlayer1 {
		if isPat2 {
			return isCrim1 // Патруль враждебен игроку только если игрок атаковал шахтера
		}
		if isM2 {
			return false // По умолчанию игроки и шахтеры мирные
		}
		return false
	}
	if isPlayer2 {
		if isPat1 {
			return isCrim2
		}
		if isM1 {
			return false
		}
		return false
	}

	// Шахтеры дружелюбны ко всем, кроме пиратов и преступников
	if isM1 {
		return isP2 || isCrim2
	}
	if isM2 {
		return isP1 || isCrim1
	}

	return false
}

// isAggressor проверяет, проявляет ли сущность id агрессию по отношению к target
func isAggressor(world *ecs.World, id, target domain.EntityID) bool {
	eType, exists := world.GetEntityType(id)
	if !exists {
		return false
	}

	// 1. Игрок является агрессором, если он явно атакует цель (у него активно оружие против нее)
	if eType == domain.EntityPlayer {
		if wVal, ok := world.GetComponent(id, domain.Weapon{}); ok {
			w := wVal.(*domain.Weapon)
			if w.Active && w.TargetID == target {
				return true
			}
		}
		return false
	}

	// 2. NPC пират агрессивен ко всем не-пиратам
	if IsPirate(world, id) {
		return !IsPirate(world, target)
	}

	// 3. NPC патруль агрессивен к пиратам и преступникам (MinerAttacker)
	if IsPatrol(world, id) {
		isPirateTarget := IsPirate(world, target)
		_, isCrimTarget := world.GetComponent(target, domain.MinerAttacker{})
		return isPirateTarget || isCrimTarget
	}

	// Шахтеры по умолчанию мирные и первыми в бой не лезут
	return false
}
