package systems

import (
	"math"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

type LootSystem struct {
	grid *spatial.HashGrid
}

func NewLootSystem(grid *spatial.HashGrid) *LootSystem {
	return &LootSystem{
		grid: grid,
	}
}

func (s *LootSystem) Name() string {
	return "LootSystem"
}

func (s *LootSystem) Priority() int {
	return 5 // Executed before cleanup
}

func (s *LootSystem) Update(world *ecs.World, dt float64) {
	// Query all loot containers
	lootMask := ecs.BuildMask(domain.Transform{}, domain.Cargo{}, domain.Loot{})
	lootEntities := world.Query(lootMask)
	if len(lootEntities) == 0 {
		return
	}

	// Query all potential pickers (Players and NPCs that have Cargo and PlayerData)
	pickerMask := ecs.BuildMask(domain.Transform{}, domain.Cargo{}, domain.PlayerData{})
	pickerEntities := world.Query(pickerMask)
	if len(pickerEntities) == 0 {
		return
	}

	pickupRadius := float64(50.0)

	for _, lootID := range lootEntities {
		lootTransVal, _ := world.GetComponent(lootID, domain.Transform{})
		lootTrans := lootTransVal.(*domain.Transform)

		lootCargoVal, _ := world.GetComponent(lootID, domain.Cargo{})
		lootCargo := lootCargoVal.(*domain.Cargo)

		lootInfoVal, _ := world.GetComponent(lootID, domain.Loot{})
		lootInfo := lootInfoVal.(*domain.Loot)

		// Check distance to all pickers
		for _, pickerID := range pickerEntities {
			pickerTransVal, _ := world.GetComponent(pickerID, domain.Transform{})
			pickerTrans := pickerTransVal.(*domain.Transform)

			dx := float64(pickerTrans.X - lootTrans.X)
			dy := float64(pickerTrans.Y - lootTrans.Y)
			dist := math.Sqrt(dx*dx + dy*dy)

			if dist <= pickupRadius {
				// 1. Pick up credits
				if lootInfo.Credits > 0 {
					pickerPlayerVal, _ := world.GetComponent(pickerID, domain.PlayerData{})
					pickerPlayer := pickerPlayerVal.(*domain.PlayerData)
					pickerPlayer.Credits += lootInfo.Credits
					lootInfo.Credits = 0
				}

				// 2. Pick up cargo items
				pickerCargoVal, _ := world.GetComponent(pickerID, domain.Cargo{})
				pickerCargo := pickerCargoVal.(*domain.Cargo)

				for i, item := range lootCargo.Items {
					if item.Quantity <= 0 {
						continue
					}
					// Calculate current picker cargo load
					pickerLoad := int32(0)
					for _, it := range pickerCargo.Items {
						pickerLoad += it.Quantity
					}
					spaceLeft := pickerCargo.Capacity - pickerLoad
					if spaceLeft <= 0 {
						break // No cargo space left on picker
					}

					toAdd := item.Quantity
					if toAdd > spaceLeft {
						toAdd = spaceLeft
					}

					pickerCargo.AddItem(item.DefinitionID, toAdd)
					lootCargo.Items[i].Quantity -= toAdd
				}

				// Clean up empty items
				var activeItems []domain.ItemInstance
				for _, item := range lootCargo.Items {
					if item.Quantity > 0 {
						activeItems = append(activeItems, item)
					}
				}
				lootCargo.Items = activeItems
			}
		}

		// Check if loot container is empty (all cargo items are empty and credits == 0)
		isEmpty := lootInfo.Credits == 0 && len(lootCargo.Items) == 0

		if isEmpty {
			if s.grid != nil {
				s.grid.Remove(lootID)
			}
			world.DestroyEntity(lootID)
		}
	}
}
