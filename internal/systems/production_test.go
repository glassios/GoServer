package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func TestRefinerySystem(t *testing.T) {
	world := ecs.NewWorld()
	refSys := NewRefinerySystem()

	station := world.CreateEntity(domain.EntityStation)
	world.AddComponent(station, &domain.Refinery{
		IsActive: true,
		Yield:    1.0,
	})
	world.AddComponent(station, &domain.Cargo{
		Items: []domain.ItemInstance{
			{DefinitionID: 1, Quantity: 10, State: "normal"},
			{DefinitionID: 2, Quantity: 5, State: "normal"},
		},
		Capacity: 100,
	})

	// Run update (dt = 1.0 second) to trigger the tick interval
	refSys.Update(world, 1.0)

	cargoVal, _ := world.GetComponent(station, domain.Cargo{})
	cargo := cargoVal.(*domain.Cargo)

	if cargo.GetResourceTypeQuantity(domain.ResourceIron) != 8 {
		t.Errorf("Expected 8 Iron remaining, got %d", cargo.GetResourceTypeQuantity(domain.ResourceIron))
	}
	if cargo.GetResourceTypeQuantity("IronPlates") != 1 {
		t.Errorf("Expected 1 IronPlates created, got %d", cargo.GetResourceTypeQuantity("IronPlates"))
	}

	if cargo.GetResourceTypeQuantity(domain.ResourceIron) != 8 { // Wait, next check is Titanium
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceTitanium) != 3 {
		t.Errorf("Expected 3 Titanium remaining, got %d", cargo.GetResourceTypeQuantity(domain.ResourceTitanium))
	}
	if cargo.GetResourceTypeQuantity("TitaniumPlates") != 1 {
		t.Errorf("Expected 1 TitaniumPlates created, got %d", cargo.GetResourceTypeQuantity("TitaniumPlates"))
	}

	refVal, _ := world.GetComponent(station, domain.Refinery{})
	ref := refVal.(*domain.Refinery)
	if !ref.IsActive {
		t.Error("Expected refinery to remain active")
	}

	// Run until resources are depleted
	refSys.Update(world, 1.0)
	refSys.Update(world, 1.0)
	refSys.Update(world, 1.0)
	refSys.Update(world, 1.0)

	if cargo.GetResourceTypeQuantity(domain.ResourceIron) > 0 {
		t.Errorf("Expected Iron to be depleted, got %d", cargo.GetResourceTypeQuantity(domain.ResourceIron))
	}
	if cargo.GetResourceTypeQuantity(domain.ResourceTitanium) != 1 {
		t.Errorf("Expected 1 Titanium remaining, got %d", cargo.GetResourceTypeQuantity(domain.ResourceTitanium))
	}

	// The refinery should now be auto-deactivated because remaining iron (0) and titanium (1) are both < 2.
	if ref.IsActive {
		t.Error("Expected refinery to auto-deactivate after raw materials are depleted")
	}
}

func TestShipyardSystem(t *testing.T) {
	world := ecs.NewWorld()
	sySys := NewShipyardSystem()

	station := world.CreateEntity(domain.EntityStation)
	world.AddComponent(station, &domain.Shipyard{
		Queue: []domain.ShipBuildOrder{
			{
				ShipType:  "fighter",
				Progress:  0.0,
				TotalTime: 3.0,
				OrderedBy: 999,
			},
		},
	})

	playerID := domain.EntityID(999)
	world.RegisterEntityWithID(playerID, domain.EntityPlayer)
	world.AddComponent(playerID, &domain.ShipConfig{
		ShipType: "miner",
		MaxSpeed: 60,
		TurnRate: 1.0,
	})
	world.AddComponent(playerID, &domain.Health{Current: 50, Max: 100})
	world.AddComponent(playerID, &domain.Shield{Current: 10, Max: 50})

	// Process shipyard (dt = 2.0 seconds)
	sySys.Update(world, 2.0)

	syVal, _ := world.GetComponent(station, domain.Shipyard{})
	sy := syVal.(*domain.Shipyard)

	if len(sy.Queue) != 1 {
		t.Fatal("Expected build order to remain in queue")
	}
	if sy.Queue[0].Progress != 2.0 {
		t.Errorf("Expected progress to be 2.0, got %f", sy.Queue[0].Progress)
	}

	// Update another 1.0 second (total 3.0, order finishes)
	sySys.Update(world, 1.0)

	if len(sy.Queue) != 0 {
		t.Fatal("Expected build order to be completed and removed from queue")
	}

	// Verify player ship upgraded and healed
	cfgVal, _ := world.GetComponent(playerID, domain.ShipConfig{})
	cfg := cfgVal.(*domain.ShipConfig)
	if cfg.ShipType != "fighter" {
		t.Errorf("Expected ship type to be fighter, got %s", cfg.ShipType)
	}
	if cfg.MaxSpeed != 120 {
		t.Errorf("Expected max speed to be 120, got %f", cfg.MaxSpeed)
	}

	hVal, _ := world.GetComponent(playerID, domain.Health{})
	h := hVal.(*domain.Health)
	if h.Current != 100 {
		t.Errorf("Expected health to be fully restored to 100, got %d", h.Current)
	}

	sVal, _ := world.GetComponent(playerID, domain.Shield{})
	s := sVal.(*domain.Shield)
	if s.Current != 50 {
		t.Errorf("Expected shield to be fully restored to 50, got %d", s.Current)
	}
}
