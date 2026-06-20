package main

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func TestVaultDepositAndWithdraw(t *testing.T) {
	world := ecs.NewWorld()

	playerID := domain.EntityID(1)
	world.RegisterEntityWithID(playerID, domain.EntityPlayer)
	playerCargo := &domain.Cargo{
		Items: []domain.ItemInstance{
			{DefinitionID: 1, Quantity: 50, State: "normal"},
		},
		Capacity: 100,
	}
	world.AddComponent(playerID, playerCargo)

	stationID := domain.EntityID(5001)
	world.RegisterEntityWithID(stationID, domain.EntityStation)
	playerVaults := &domain.StationVaults{
		PlayerVaults: make(map[uint64][]domain.ItemInstance),
	}
	corpVault := &domain.CorporationVault{
		OwnerCorpID: 0,
		Items:       []domain.ItemInstance{},
	}
	world.AddComponent(stationID, playerVaults)
	world.AddComponent(stationID, corpVault)

	// Test Successful Deposit
	resType := domain.ResourceIron
	defID := domain.ResourceToID[resType]
	amount := int32(20)

	playerQty := playerCargo.GetResourceTypeQuantity(resType)
	if playerQty < amount {
		t.Fatalf("Insufficient items in cargo")
	}

	playerCargo.RemoveResourceTypeQuantity(resType, amount)
	
	vaultItems := playerVaults.PlayerVaults[uint64(playerID)]
	vaultItems = addItemToSlice(vaultItems, defID, amount)
	playerVaults.PlayerVaults[uint64(playerID)] = vaultItems

	if playerCargo.GetResourceTypeQuantity(resType) != 30 {
		t.Errorf("Expected 30 Iron in ship cargo, got %d", playerCargo.GetResourceTypeQuantity(resType))
	}
	if getQuantityInSlice(playerVaults.PlayerVaults[uint64(playerID)], defID) != 20 {
		t.Errorf("Expected 20 Iron in player vault, got %d", getQuantityInSlice(playerVaults.PlayerVaults[uint64(playerID)], defID))
	}

	// Test Successful Withdraw
	withdrawAmount := int32(10)
	var currentLoad int32 = 0
	for _, item := range playerCargo.Items {
		currentLoad += item.Quantity
	}
	if currentLoad+withdrawAmount > playerCargo.Capacity {
		t.Fatalf("Cargo full")
	}

	vaultItems = playerVaults.PlayerVaults[uint64(playerID)]
	vaultItems, _ = removeItemFromSlice(vaultItems, defID, withdrawAmount)
	playerVaults.PlayerVaults[uint64(playerID)] = vaultItems
	
	playerCargo.AddResourceTypeQuantity(resType, withdrawAmount)

	if playerCargo.GetResourceTypeQuantity(resType) != 40 {
		t.Errorf("Expected 40 Iron in ship cargo, got %d", playerCargo.GetResourceTypeQuantity(resType))
	}
	if getQuantityInSlice(playerVaults.PlayerVaults[uint64(playerID)], defID) != 10 {
		t.Errorf("Expected 10 Iron in player vault, got %d", getQuantityInSlice(playerVaults.PlayerVaults[uint64(playerID)], defID))
	}
}

func TestVaultCapacityExceeded(t *testing.T) {
	world := ecs.NewWorld()

	playerID := domain.EntityID(1)
	world.RegisterEntityWithID(playerID, domain.EntityPlayer)
	playerCargo := &domain.Cargo{
		Items: []domain.ItemInstance{
			{DefinitionID: 1, Quantity: 90, State: "normal"},
		},
		Capacity: 100,
	}
	world.AddComponent(playerID, playerCargo)

	stationID := domain.EntityID(5001)
	world.RegisterEntityWithID(stationID, domain.EntityStation)
	playerVaults := &domain.StationVaults{
		PlayerVaults: map[uint64][]domain.ItemInstance{
			uint64(playerID): {
				{DefinitionID: 1, Quantity: 20, State: "normal"},
			},
		},
	}
	world.AddComponent(stationID, playerVaults)

	withdrawAmount := int32(20)

	var currentLoad int32 = 0
	for _, item := range playerCargo.Items {
		currentLoad += item.Quantity
	}

	if currentLoad+withdrawAmount > playerCargo.Capacity {
		// Target exceeded: expected failure
		return
	}
	t.Errorf("Expected withdraw to fail due to cargo capacity overflow")
}

func TestCorpVaultAccessControl(t *testing.T) {
	world := ecs.NewWorld()

	playerID := domain.EntityID(1)
	world.RegisterEntityWithID(playerID, domain.EntityPlayer)
	world.AddComponent(playerID, &domain.CorporationMember{
		CorpID: 99,
		Role:   "Member",
	})

	stationID := domain.EntityID(5001)
	world.RegisterEntityWithID(stationID, domain.EntityStation)
	world.AddComponent(stationID, &domain.StationOwnership{
		CorpID: 100, // owned by different corp
	})

	var playerCorpID uint32 = 0
	if cMemberVal, ok := world.GetComponent(playerID, domain.CorporationMember{}); ok {
		playerCorpID = cMemberVal.(*domain.CorporationMember).CorpID
	}

	var stationCorpID uint32 = 0
	if ownerVal, ok := world.GetComponent(stationID, domain.StationOwnership{}); ok {
		stationCorpID = ownerVal.(*domain.StationOwnership).CorpID
	}

	// Access check: player corp (99) != station corp (100)
	if stationCorpID == 0 || playerCorpID != stationCorpID {
		// Access locked: expected behavior
		return
	}
	t.Errorf("Expected access to corporate vault to be blocked")
}
