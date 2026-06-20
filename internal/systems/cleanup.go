package systems

import (
	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

type CleanupSystem struct {
	grid *spatial.HashGrid
}

func NewCleanupSystem(grid *spatial.HashGrid) *CleanupSystem {
	return &CleanupSystem{
		grid: grid,
	}
}

func (s *CleanupSystem) Name() string {
	return "CleanupSystem"
}

func (s *CleanupSystem) Priority() int {
	return 0 // Executed last in the tick
}

func (s *CleanupSystem) Update(world *ecs.World, dt float64) {
	// 1. Identify entities to destroy
	var entitiesToDestroy []domain.EntityID

	// Check Health
	hMask := ecs.BuildMask(domain.Health{})
	hEntities := world.Query(hMask)
	for _, id := range hEntities {
		hVal, _ := world.GetComponent(id, domain.Health{})
		if hVal.(*domain.Health).Current <= 0 {
			entitiesToDestroy = append(entitiesToDestroy, id)
		}
	}

	// Check Asteroids resource exhaustion
	rMask := ecs.BuildMask(domain.AsteroidResource{})
	rEntities := world.Query(rMask)
	for _, id := range rEntities {
		rVal, _ := world.GetComponent(id, domain.AsteroidResource{})
		if rVal.(*domain.AsteroidResource).Amount <= 0 {
			// Ensure we don't duplicate
			alreadyAdded := false
			for _, d := range entitiesToDestroy {
				if d == id {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				entitiesToDestroy = append(entitiesToDestroy, id)
			}
		}
	}

	// Destroy entities
	for _, id := range entitiesToDestroy {
		eType, exists := world.GetEntityType(id)
		if exists && (eType == domain.EntityPlayer || eType == domain.EntityNPC) {
			// Extract cargo and credits
			var credits int64
			var hasLoot bool
			var items []domain.ItemInstance

			if pVal, ok := world.GetComponent(id, domain.PlayerData{}); ok {
				credits = pVal.(*domain.PlayerData).Credits
				if credits > 0 {
					hasLoot = true
				}
			}

			if cargoVal, ok := world.GetComponent(id, domain.Cargo{}); ok {
				cargo := cargoVal.(*domain.Cargo)
				for _, item := range cargo.Items {
					if item.Quantity > 0 {
						items = append(items, item)
						hasLoot = true
					}
				}
			}

			// Проверяем, находится ли сущность в боевом инстансе (имеет компонент CombatTeam).
			// Если да, то лут в комнате боя НЕ выпадает.
			_, inCombatInstance := world.GetComponent(id, domain.CombatTeam{})

			if hasLoot && !inCombatInstance {
				var posX, posY float32
				if tVal, ok := world.GetComponent(id, domain.Transform{}); ok {
					t := tVal.(*domain.Transform)
					posX = t.X
					posY = t.Y
				}

				lootEntity := world.CreateEntity(domain.EntityLootContainer)
				world.AddComponent(lootEntity, &domain.Transform{X: posX, Y: posY})
				world.AddComponent(lootEntity, &domain.Loot{Credits: credits})

				lootCargo := &domain.Cargo{
					Capacity: 999999,
					Items:    items,
				}
				world.AddComponent(lootEntity, lootCargo)

				if s.grid != nil {
					s.grid.Insert(lootEntity, posX, posY)
				}
			}
		}

		if s.grid != nil {
			s.grid.Remove(id)
		}
		world.DestroyEntity(id)
	}

	// 2. Clean up invalid TargetIDs in remaining entities
	// Clean Weapon targets
	wMask := ecs.BuildMask(domain.Weapon{})
	wEntities := world.Query(wMask)
	for _, id := range wEntities {
		wVal, _ := world.GetComponent(id, domain.Weapon{})
		weapon := wVal.(*domain.Weapon)
		if weapon.Active {
			if _, exists := world.GetEntityType(weapon.TargetID); !exists {
				weapon.Active = false
				weapon.TargetID = 0
			}
		}
	}

	// Clean MiningLaser targets
	lMask := ecs.BuildMask(domain.MiningLaser{})
	lEntities := world.Query(lMask)
	for _, id := range lEntities {
		lVal, _ := world.GetComponent(id, domain.MiningLaser{})
		laser := lVal.(*domain.MiningLaser)
		if laser.Active {
			if _, exists := world.GetEntityType(laser.TargetID); !exists {
				laser.Active = false
				laser.TargetID = 0
			}
		}
	}

	// Clean AIState targets
	aiMask := ecs.BuildMask(domain.AIState{})
	aiEntities := world.Query(aiMask)
	for _, id := range aiEntities {
		aiVal, _ := world.GetComponent(id, domain.AIState{})
		ai := aiVal.(*domain.AIState)
		if ai.TargetID != 0 {
			if _, exists := world.GetEntityType(ai.TargetID); !exists {
				ai.TargetID = 0
				if _, inCombat := world.GetComponent(id, domain.CombatTeam{}); !inCombat {
					ai.Behavior = domain.BehaviorIdle
				}
			}
		}
	}
}
