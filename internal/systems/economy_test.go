package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func TestEconomy_Pricing(t *testing.T) {
	item := &domain.MarketItem{
		BasePrice: 100,
		Supply:    50,
		Demand:    50,
	}

	// Supply == Demand -> price should be close to BasePrice
	priceInitBuy := CalculatePrice(item, true)
	priceInitSell := CalculatePrice(item, false)

	if priceInitBuy <= priceInitSell {
		t.Error("expected buy price to be higher than sell price due to spread")
	}

	// Increase Demand
	item.Demand = 150
	priceHighDemand := CalculatePrice(item, true)
	if priceHighDemand <= priceInitBuy {
		t.Errorf("expected price to rise on high demand, got initial %d and high %d", priceInitBuy, priceHighDemand)
	}

	// Increase Supply
	item.Supply = 300
	priceHighSupply := CalculatePrice(item, true)
	if priceHighSupply >= priceHighDemand {
		t.Errorf("expected price to fall on high supply, got high demand %d and high supply %d", priceHighDemand, priceHighSupply)
	}
}

func TestEconomy_Trading(t *testing.T) {
	world := ecs.NewWorld()

	player := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(player, &domain.PlayerData{Credits: 1000, Name: "Trader"})
	world.AddComponent(player, &domain.Cargo{Capacity: 100, Items: []domain.ItemInstance{}})

	station := world.CreateEntity(domain.EntityStation)
	market := &domain.StationMarket{
		Items: map[domain.ResourceType]*domain.MarketItem{
			domain.ResourceIron: {
				BasePrice: 10,
				Supply:    50,
				Demand:    20,
			},
		},
	}
	world.AddComponent(station, market)

	// Get initial price
	ironItem := market.Items[domain.ResourceIron]
	initialBuyPrice := CalculatePrice(ironItem, true)

	// 1. Buy resource
	err := ExecuteTrade(world, player, station, domain.ResourceIron, 10, true)
	if err != nil {
		t.Fatalf("failed to buy resource: %v", err)
	}

	// Verify player cargo and credits
	pDataVal, _ := world.GetComponent(player, domain.PlayerData{})
	pCargoVal, _ := world.GetComponent(player, domain.Cargo{})
	pData := pDataVal.(*domain.PlayerData)
	cargo := pCargoVal.(*domain.Cargo)

	expectedCost := int64(initialBuyPrice) * 10
	if pData.Credits != 1000-expectedCost {
		t.Errorf("expected credits %d, got %d", 1000-expectedCost, pData.Credits)
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceIron) != 10 {
		t.Errorf("expected 10 iron in cargo, got %d", cargo.GetResourceTypeQuantity(domain.ResourceIron))
	}

	// Price should have increased because supply dropped
	newBuyPrice := CalculatePrice(ironItem, true)
	if newBuyPrice <= initialBuyPrice {
		t.Errorf("expected price to rise after purchase, old: %d, new: %d", initialBuyPrice, newBuyPrice)
	}

	// 2. Buy with insufficient credits
	pData.Credits = 5 // set credits very low
	err = ExecuteTrade(world, player, station, domain.ResourceIron, 5, true)
	if err != domain.ErrInsufficientCredits {
		t.Errorf("expected ErrInsufficientCredits, got %v", err)
	}

	// Restore credits
	pData.Credits = 1000

	// 3. Buy exceeding cargo capacity
	cargo.Capacity = 15 // only 5 slots left (since 10 are filled)
	err = ExecuteTrade(world, player, station, domain.ResourceIron, 10, true)
	if err != domain.ErrCargoFull {
		t.Errorf("expected ErrCargoFull, got %v", err)
	}

	// Restore capacity
	cargo.Capacity = 100

	// 4. Sell resource back to station
	initialSellPrice := CalculatePrice(ironItem, false)
	err = ExecuteTrade(world, player, station, domain.ResourceIron, 5, false)
	if err != nil {
		t.Fatalf("failed to sell resource: %v", err)
	}

	expectedProfit := int64(initialSellPrice) * 5
	if pData.Credits != 1000+expectedProfit {
		t.Errorf("expected credits %d, got %d", 1000+expectedProfit, pData.Credits)
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceIron) != 5 {
		t.Errorf("expected 5 iron in cargo after selling 5, got %d", cargo.GetResourceTypeQuantity(domain.ResourceIron))
	}
}
