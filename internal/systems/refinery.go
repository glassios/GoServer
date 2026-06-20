package systems

import (
	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

type RefinerySystem struct {
	tickInterval  float64
	timeSinceLast float64
}

func NewRefinerySystem() *RefinerySystem {
	return &RefinerySystem{
		tickInterval: 1.0, // process once per second
	}
}

func (s *RefinerySystem) Name() string {
	return "RefinerySystem"
}

func (s *RefinerySystem) Priority() int {
	return 80
}

func (s *RefinerySystem) Update(world *ecs.World, dt float64) {
	s.timeSinceLast += dt
	if s.timeSinceLast < s.tickInterval {
		return
	}
	s.timeSinceLast = 0.0

	mask := ecs.BuildMask(domain.Refinery{}, domain.Cargo{})
	entities := world.Query(mask)

	for _, stationID := range entities {
		refVal, ok1 := world.GetComponent(stationID, domain.Refinery{})
		cargoVal, ok2 := world.GetComponent(stationID, domain.Cargo{})
		if !ok1 || !ok2 {
			continue
		}

		ref := refVal.(*domain.Refinery)
		cargo := cargoVal.(*domain.Cargo)

		if !ref.IsActive {
			continue
		}

		marketVal, ok3 := world.GetComponent(stationID, domain.StationMarket{})

		// 1. Tier 1: Refining (Ores -> Materials)
		// Convert Iron -> IronPlates
		if cargo.GetResourceTypeQuantity(domain.ResourceIron) >= 2 {
			cargo.RemoveResourceTypeQuantity(domain.ResourceIron, 2)
			cargo.AddResourceTypeQuantity("IronPlates", 1)
			if ok3 {
				market := marketVal.(*domain.StationMarket)
				if ironItem, exists := market.Items[domain.ResourceIron]; exists {
					if ironItem.Supply >= 2 {
						ironItem.Supply -= 2
					} else {
						ironItem.Supply = 0
					}
				}
			}
		}

		// Convert Titanium -> TitaniumPlates
		if cargo.GetResourceTypeQuantity(domain.ResourceTitanium) >= 2 {
			cargo.RemoveResourceTypeQuantity(domain.ResourceTitanium, 2)
			cargo.AddResourceTypeQuantity("TitaniumPlates", 1)
			if ok3 {
				market := marketVal.(*domain.StationMarket)
				if titaniumItem, exists := market.Items[domain.ResourceTitanium]; exists {
					if titaniumItem.Supply >= 2 {
						titaniumItem.Supply -= 2
					} else {
						titaniumItem.Supply = 0
					}
				}
			}
		}

		// Convert Crystal -> SiliconWafers
		if cargo.GetResourceTypeQuantity(domain.ResourceCrystal) >= 2 {
			cargo.RemoveResourceTypeQuantity(domain.ResourceCrystal, 2)
			cargo.AddResourceTypeQuantity(domain.ResourceSiliconWafers, 1)
			if ok3 {
				market := marketVal.(*domain.StationMarket)
				if crystalItem, exists := market.Items[domain.ResourceCrystal]; exists {
					if crystalItem.Supply >= 2 {
						crystalItem.Supply -= 2
					} else {
						crystalItem.Supply = 0
					}
				}
			}
		}

		// Convert RareGas -> FuelCells
		if cargo.GetResourceTypeQuantity(domain.ResourceRareGas) >= 2 {
			cargo.RemoveResourceTypeQuantity(domain.ResourceRareGas, 2)
			cargo.AddResourceTypeQuantity(domain.ResourceFuelCells, 1)
			if ok3 {
				market := marketVal.(*domain.StationMarket)
				if gasItem, exists := market.Items[domain.ResourceRareGas]; exists {
					if gasItem.Supply >= 2 {
						gasItem.Supply -= 2
					} else {
						gasItem.Supply = 0
					}
				}
			}
		}

		// 2. Tier 2: Components (Materials -> Components)
		// SiliconWafers + TitaniumPlates -> Microchips
		if cargo.GetResourceTypeQuantity(domain.ResourceSiliconWafers) >= 1 && cargo.GetResourceTypeQuantity("TitaniumPlates") >= 1 {
			cargo.RemoveResourceTypeQuantity(domain.ResourceSiliconWafers, 1)
			cargo.RemoveResourceTypeQuantity("TitaniumPlates", 1)
			cargo.AddResourceTypeQuantity(domain.ResourceMicrochips, 1)
		}

		// FuelCells + IronPlates -> EnergyCoils
		if cargo.GetResourceTypeQuantity(domain.ResourceFuelCells) >= 1 && cargo.GetResourceTypeQuantity("IronPlates") >= 1 {
			cargo.RemoveResourceTypeQuantity(domain.ResourceFuelCells, 1)
			cargo.RemoveResourceTypeQuantity("IronPlates", 1)
			cargo.AddResourceTypeQuantity(domain.ResourceEnergyCoils, 1)
		}

		// 3. Tier 3: Assembly (Components -> Weapons/Modules)
		// 2 Microchips + 2 EnergyCoils -> Laser Cannon
		if cargo.GetResourceTypeQuantity(domain.ResourceMicrochips) >= 2 && cargo.GetResourceTypeQuantity(domain.ResourceEnergyCoils) >= 2 {
			cargo.RemoveResourceTypeQuantity(domain.ResourceMicrochips, 2)
			cargo.RemoveResourceTypeQuantity(domain.ResourceEnergyCoils, 2)
			cargo.AddResourceTypeQuantity("Laser Cannon", 1)
		}

		// 2 Microchips + 2 IronPlates -> Mining Laser
		if cargo.GetResourceTypeQuantity(domain.ResourceMicrochips) >= 2 && cargo.GetResourceTypeQuantity("IronPlates") >= 2 {
			cargo.RemoveResourceTypeQuantity(domain.ResourceMicrochips, 2)
			cargo.RemoveResourceTypeQuantity("IronPlates", 2)
			cargo.AddResourceTypeQuantity("Mining Laser", 1)
		}

		// If no more materials can be processed for the next tick, automatically stop
		if !canRefineOrCraft(cargo) {
			ref.IsActive = false
		}
	}
}

func canRefineOrCraft(cargo *domain.Cargo) bool {
	// Tier 1: Refining
	if cargo.GetResourceTypeQuantity(domain.ResourceIron) >= 2 {
		return true
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceTitanium) >= 2 {
		return true
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceCrystal) >= 2 {
		return true
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceRareGas) >= 2 {
		return true
	}

	// Tier 2: Components
	if cargo.GetResourceTypeQuantity(domain.ResourceSiliconWafers) >= 1 && cargo.GetResourceTypeQuantity("TitaniumPlates") >= 1 {
		return true
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceFuelCells) >= 1 && cargo.GetResourceTypeQuantity("IronPlates") >= 1 {
		return true
	}

	// Tier 3: Assembly
	if cargo.GetResourceTypeQuantity(domain.ResourceMicrochips) >= 2 && cargo.GetResourceTypeQuantity(domain.ResourceEnergyCoils) >= 2 {
		return true
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceMicrochips) >= 2 && cargo.GetResourceTypeQuantity("IronPlates") >= 2 {
		return true
	}

	return false
}
