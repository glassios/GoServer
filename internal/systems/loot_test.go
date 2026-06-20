package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

func TestLootSystem_DropAndPickup(t *testing.T) {
	world := ecs.NewWorld()
	grid := spatial.NewHashGrid(100.0)

	cleanupSys := NewCleanupSystem(grid)
	lootSys := NewLootSystem(grid)

	// 1. Create a dying pirate NPC with some cargo and credits
	pirate := world.CreateEntity(domain.EntityNPC)
	world.AddComponent(pirate, &domain.Transform{X: 10, Y: 10})
	world.AddComponent(pirate, &domain.Health{Current: 0, Max: 100}) // Health is 0 so it will be cleaned up
	world.AddComponent(pirate, &domain.ShipConfig{ShipType: "pirate", MaxSpeed: 50})
	world.AddComponent(pirate, &domain.PlayerData{Name: "Pirate Bot", Credits: 300})
	
	pirateCargo := &domain.Cargo{
		Capacity: 50,
		Items: []domain.ItemInstance{
			{DefinitionID: 1, Quantity: 20, State: "normal"},
			{DefinitionID: 2, Quantity: 10, State: "normal"},
		},
	}
	world.AddComponent(pirate, pirateCargo)

	grid.Insert(pirate, 10, 10)

	// Run cleanup system -> should destroy the pirate and spawn a LootContainer at X: 10, Y: 10
	cleanupSys.Update(world, 0.05)

	// Verify pirate is gone
	_, exists := world.GetEntityType(pirate)
	if exists {
		t.Fatal("Pirate entity should have been destroyed by cleanup system")
	}

	// Verify a LootContainer was spawned
	lootMask := ecs.BuildMask(domain.Transform{}, domain.Cargo{}, domain.Loot{})
	lootEntities := world.Query(lootMask)
	if len(lootEntities) != 1 {
		t.Fatalf("Expected exactly 1 loot container spawned, got %d", len(lootEntities))
	}

	lootID := lootEntities[0]
	lType, lExists := world.GetEntityType(lootID)
	if !lExists || lType != domain.EntityLootContainer {
		t.Fatalf("Expected entity type to be EntityLootContainer, got %v", lType)
	}

	// Verify loot contents
	lootInfoVal, _ := world.GetComponent(lootID, domain.Loot{})
	lootInfo := lootInfoVal.(*domain.Loot)
	if lootInfo.Credits != 300 {
		t.Errorf("Expected 300 credits in loot container, got %d", lootInfo.Credits)
	}

	lootCargoVal, _ := world.GetComponent(lootID, domain.Cargo{})
	lootCargo := lootCargoVal.(*domain.Cargo)
	if lootCargo.GetResourceTypeQuantity(domain.ResourceIron) != 20 || lootCargo.GetResourceTypeQuantity(domain.ResourceTitanium) != 10 {
		t.Errorf("Loot container cargo items mismatch: %+v", lootCargo.Items)
	}

	// 2. Create a Player picker close enough (X: 12, Y: 12) with limited cargo space (Capacity: 15)
	player := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(player, &domain.Transform{X: 12, Y: 12})
	world.AddComponent(player, &domain.PlayerData{Name: "Picker Player", Credits: 50})
	
	playerCargo := &domain.Cargo{Capacity: 15, Items: []domain.ItemInstance{}}
	world.AddComponent(player, playerCargo)

	// Run loot system update
	lootSys.Update(world, 0.05)

	// Verify credits were transferred completely
	playerInfoVal, _ := world.GetComponent(player, domain.PlayerData{})
	playerInfo := playerInfoVal.(*domain.PlayerData)
	if playerInfo.Credits != 350 {
		t.Errorf("Expected player credits to be 350, got %d", playerInfo.Credits)
	}
	if lootInfo.Credits != 0 {
		t.Errorf("Expected loot credits to be 0 after pickup, got %d", lootInfo.Credits)
	}

	// Verify cargo transfer was capped by player cargo capacity (15)
	playerCargoVal, _ := world.GetComponent(player, domain.Cargo{})
	playerCargo = playerCargoVal.(*domain.Cargo)
	playerTotal := playerCargo.GetResourceTypeQuantity(domain.ResourceIron) + playerCargo.GetResourceTypeQuantity(domain.ResourceTitanium)
	if playerTotal != 15 {
		t.Errorf("Expected player to pick up exactly 15 items in total, got %d (Iron: %d, Titanium: %d)",
			playerTotal, playerCargo.GetResourceTypeQuantity(domain.ResourceIron), playerCargo.GetResourceTypeQuantity(domain.ResourceTitanium))
	}

	lootTotal := lootCargo.GetResourceTypeQuantity(domain.ResourceIron) + lootCargo.GetResourceTypeQuantity(domain.ResourceTitanium)
	if lootTotal != 15 {
		t.Errorf("Expected 15 items remaining in loot container, got %d (Iron: %d, Titanium: %d)",
			lootTotal, lootCargo.GetResourceTypeQuantity(domain.ResourceIron), lootCargo.GetResourceTypeQuantity(domain.ResourceTitanium))
	}

	// Verify loot container is NOT destroyed because it still has items
	_, exists = world.GetEntityType(lootID)
	if !exists {
		t.Fatal("Loot container should not have been destroyed as it is not empty")
	}

	// 3. Upgrade player's cargo capacity to 50 and update loot system -> remainder should be picked up
	playerCargo.Capacity = 50
	lootSys.Update(world, 0.05)

	// Player should have 20 Iron and 10 Titanium now
	if playerCargo.GetResourceTypeQuantity(domain.ResourceIron) != 20 || playerCargo.GetResourceTypeQuantity(domain.ResourceTitanium) != 10 {
		t.Errorf("Expected player cargo to be full (20 Iron, 10 Titanium), got %+v", playerCargo.Items)
	}

	// Loot container should be empty and destroyed
	_, exists = world.GetEntityType(lootID)
	if exists {
		t.Fatal("Loot container should have been destroyed once empty")
	}
}
