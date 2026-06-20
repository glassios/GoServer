package systems

import (
	"math"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

type EconomySystem struct {
	accumulatedTime float64
}

func NewEconomySystem() *EconomySystem {
	return &EconomySystem{}
}

func (s *EconomySystem) Name() string {
	return "EconomySystem"
}

func (s *EconomySystem) Priority() int {
	return 20 // Runs after combat/mining
}

func (s *EconomySystem) Update(world *ecs.World, dt float64) {
	s.accumulatedTime += dt

	// Simulate economy change over time (e.g., consumption every 5 seconds)
	if s.accumulatedTime >= 5.0 {
		s.accumulatedTime = 0

		mask := ecs.BuildMask(domain.StationMarket{})
		stations := world.Query(mask)

		for _, id := range stations {
			mVal, found := world.GetComponent(id, domain.StationMarket{})
			if !found {
				continue
			}

			market := mVal.(*domain.StationMarket)

			// Stations consume resources, increasing demand and decreasing supply slowly
			for _, item := range market.Items {
				item.Demand += 1
				if item.Supply > 0 {
					item.Supply -= 1
				}
			}
		}
	}
}

// CalculatePrice computes the current dynamic price of an item.
func CalculatePrice(item *domain.MarketItem, isBuy bool) int32 {
	if item.BasePrice <= 0 {
		return 1
	}

	// price = BasePrice * (1 + (Demand - Supply) / (Demand + Supply + 1))
	total := float64(item.Demand + item.Supply + 1)
	factor := float64(item.Demand-item.Supply) / total

	price := float64(item.BasePrice) * (1.0 + factor)

	// Apply spread (buy price is higher, sell price is lower)
	if isBuy {
		price *= 1.05
	} else {
		price *= 0.95
	}

	intPrice := int32(math.Round(price))

	// Clamping
	if intPrice < 1 {
		return 1
	}
	if intPrice > item.BasePrice*5 {
		return item.BasePrice * 5
	}

	return intPrice
}

// ExecuteTrade handles resource trading between Player and Station.
func ExecuteTrade(world *ecs.World, playerID, stationID domain.EntityID, resource domain.ResourceType, amount int32, isBuy bool) error {
	pDataVal, foundP := world.GetComponent(playerID, domain.PlayerData{})
	pCargoVal, foundC := world.GetComponent(playerID, domain.Cargo{})
	sMarketVal, foundM := world.GetComponent(stationID, domain.StationMarket{})

	if !foundP || !foundC || !foundM {
		return domain.ErrInvalidTarget
	}

	player := pDataVal.(*domain.PlayerData)
	cargo := pCargoVal.(*domain.Cargo)
	market := sMarketVal.(*domain.StationMarket)

	item, exists := market.Items[resource]
	if !exists {
		return domain.ErrInvalidTarget
	}

	price := CalculatePrice(item, isBuy)
	totalCost := int64(price) * int64(amount)

	sCargoVal, foundSC := world.GetComponent(stationID, domain.Cargo{})

	if isBuy {
		// Player buys from Station
		if player.Credits < totalCost {
			return domain.ErrInsufficientCredits
		}
		if item.Supply < amount {
			return domain.ErrOutOfRange // Not enough stock on station
		}

		// Check cargo space
		currentVolume := int32(0)
		for _, it := range cargo.Items {
			currentVolume += it.Quantity
		}
		if currentVolume+amount > cargo.Capacity {
			return domain.ErrCargoFull
		}

		// Perform trade
		player.Credits -= totalCost
		item.Supply -= amount
		item.Demand += amount / 2 // buying increases demand factor
		
		cargo.AddResourceTypeQuantity(resource, amount)

		if foundSC {
			stationCargo := sCargoVal.(*domain.Cargo)
			stationCargo.RemoveResourceTypeQuantity(resource, amount)
		}
	} else {
		// Player sells to Station
		currentQty := cargo.GetResourceTypeQuantity(resource)
		if currentQty < amount {
			return domain.ErrInvalidTarget // Player doesn't have enough resources
		}

		// Perform trade
		player.Credits += totalCost
		item.Supply += amount
		if item.Demand > amount/2 {
			item.Demand -= amount / 2 // selling decreases demand factor
		} else {
			item.Demand = 0
		}

		cargo.RemoveResourceTypeQuantity(resource, amount)

		if foundSC {
			stationCargo := sCargoVal.(*domain.Cargo)
			stationCargo.AddResourceTypeQuantity(resource, amount)
		}
	}

	return nil
}
