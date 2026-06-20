package systems

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

func TestJumpGateSystem_PlayerMigration(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 1. Shared Messaging Bus
	bus := messaging.NewMockMessageBus()
	defer bus.Close()

	// 2. Setup System 1 (Source)
	sys1ID := uint32(1)
	world1 := ecs.NewWorld()
	grid1 := spatial.NewHashGrid(100)
	jgSys1 := NewJumpGateSystem(bus, sys1ID, logger)

	// Create a Jump Gate in System 1
	gate := world1.CreateEntity(domain.EntityJumpGate)
	world1.AddComponent(gate, &domain.Transform{X: 2000, Y: 2000})
	world1.AddComponent(gate, &domain.JumpGate{
		TargetSystemID: 2,
		TargetX:        -1800,
		TargetY:        -1800,
	})
	grid1.Insert(gate, 2000, 2000)

	// Create a Player in System 1 near the Gate
	playerID := domain.EntityID(999)
	world1.RegisterEntityWithID(playerID, domain.EntityPlayer)
	world1.AddComponent(playerID, &domain.Transform{X: 1950, Y: 1950}) // Distance ~70.7 units (< 100 threshold)
	world1.AddComponent(playerID, &domain.Velocity{X: 10, Y: -5})
	world1.AddComponent(playerID, &domain.Health{Current: 80, Max: 100})
	world1.AddComponent(playerID, &domain.Shield{Current: 40, Max: 50, RegenRate: 1.5})
	world1.AddComponent(playerID, &domain.ShipConfig{ShipType: "interceptor", MaxSpeed: 120, TurnRate: 2.0})
	world1.AddComponent(playerID, &domain.PlayerData{AccountID: 12345, Name: "Starfarer", Credits: 1500})
	world1.AddComponent(playerID, &domain.FactionMember{FactionID: 3})
	world1.AddComponent(playerID, &domain.Cargo{
		Items: []domain.ItemInstance{
			{DefinitionID: 1, Quantity: 15, State: "normal"},
		},
		Capacity: 80,
	})
	world1.AddComponent(playerID, &domain.Weapon{Type: domain.WeaponPlasma, Damage: 25, Range: 400, Cooldown: 1.0})
	world1.AddComponent(playerID, &domain.MiningLaser{Power: 2, Range: 200})
	grid1.Insert(playerID, 1950, 1950)

	// 3. Setup System 2 (Target)
	sys2ID := uint32(2)
	world2 := ecs.NewWorld()
	grid2 := spatial.NewHashGrid(100)

	// Register migration handler on System 2
	_, err := RegisterMigrationHandler(bus, world2, grid2, sys2ID, logger)
	if err != nil {
		t.Fatalf("Failed to register migration handler on sys 2: %v", err)
	}

	// 4. Capture Routing Table Update
	routingUpdated := make(chan string, 1)
	_, err = bus.Subscribe("system.routing.update", func(msg *messaging.Message) {
		routingUpdated <- string(msg.Data)
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to routing updates: %v", err)
	}

	// 5. Run JumpGateSystem Update on System 1
	jgSys1.Update(world1, 0.05)

	// Wait for async events to finish propagation (using mock bus goroutines)
	time.Sleep(50 * time.Millisecond)

	// 6. Verify Player Deleted from System 1
	_, foundInSys1 := world1.GetEntityType(playerID)
	if foundInSys1 {
		t.Error("Expected player to be removed from source system world, but they still exist")
	}

	// 7. Verify Player Created in System 2
	_, foundInSys2 := world2.GetEntityType(playerID)
	if !foundInSys2 {
		t.Fatal("Expected player to be created in target system world, but they were not found")
	}

	// 8. Verify Component Retention in System 2
	tVal, _ := world2.GetComponent(playerID, domain.Transform{})
	trans := tVal.(*domain.Transform)
	if trans.X != -1800 || trans.Y != -1800 {
		t.Errorf("Expected spawn coordinates (-1800, -1800), got (%.1f, %.1f)", trans.X, trans.Y)
	}

	hVal, _ := world2.GetComponent(playerID, domain.Health{})
	health := hVal.(*domain.Health)
	if health.Current != 80 || health.Max != 100 {
		t.Errorf("Expected health 80/100, got %d/%d", health.Current, health.Max)
	}

	cVal, _ := world2.GetComponent(playerID, domain.Cargo{})
	cargo := cVal.(*domain.Cargo)
	if cargo.GetResourceTypeQuantity(domain.ResourceIron) != 15 || cargo.Capacity != 80 {
		t.Errorf("Expected cargo Iron count 15, got %d", cargo.GetResourceTypeQuantity(domain.ResourceIron))
	}

	pVal, _ := world2.GetComponent(playerID, domain.PlayerData{})
	playerData := pVal.(*domain.PlayerData)
	if playerData.Name != "Starfarer" || playerData.Credits != 1500 {
		t.Errorf("Expected player name 'Starfarer' and credits 1500, got '%s' and %d", playerData.Name, playerData.Credits)
	}

	// 9. Verify Routing Update Notification
	select {
	case update := <-routingUpdated:
		expected := fmt.Sprintf("%d,%d", playerID, sys2ID)
		if update != expected {
			t.Errorf("Expected routing update message '%s', got '%s'", expected, update)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timed out waiting for routing table update notification on messaging bus")
	}
}

func TestAISystem_MinerCrossSystemTradeRoute(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 1. Shared Messaging Bus
	bus := messaging.NewMockMessageBus()
	defer bus.Close()

	// 2. Setup System 1 (Source)
	sys1ID := uint32(1)
	world1 := ecs.NewWorld()
	grid1 := spatial.NewHashGrid(100)
	jgSys1 := NewJumpGateSystem(bus, sys1ID, logger)
	aiSys1 := NewAISystem(500.0, 10, 10000, 10000)
	moveSys1 := NewMovementSystem(10000, 10000)

	// Seeding System 1: Asteroid and Jump Gate (no stations)
	ast := world1.CreateEntity(domain.EntityAsteroid)
	world1.AddComponent(ast, &domain.Transform{X: 100, Y: 100})
	world1.AddComponent(ast, &domain.AsteroidResource{Type: domain.ResourceIron, Amount: 1000})

	gate1 := world1.CreateEntity(domain.EntityJumpGate)
	world1.AddComponent(gate1, &domain.Transform{X: 2000, Y: 2000})
	world1.AddComponent(gate1, &domain.JumpGate{
		TargetSystemID: 2,
		TargetX:        -1800,
		TargetY:        -1800,
	})
	grid1.Insert(gate1, 2000, 2000)

	// Spawn Miner in System 1
	minerID := domain.EntityID(111)
	world1.RegisterEntityWithID(minerID, domain.EntityNPC)
	world1.AddComponent(minerID, &domain.Transform{X: 120, Y: 120})
	world1.AddComponent(minerID, &domain.Velocity{X: 0, Y: 0})
	world1.AddComponent(minerID, &domain.ShipConfig{ShipType: "miner", MaxSpeed: 100})
	world1.AddComponent(minerID, &domain.AIState{Behavior: domain.BehaviorIdle})
	world1.AddComponent(minerID, &domain.FactionMember{FactionID: 2})
	world1.AddComponent(minerID, &domain.Cargo{Capacity: 50, Items: []domain.ItemInstance{}})
	world1.AddComponent(minerID, &domain.MiningLaser{Power: 10, Range: 50})
	grid1.Insert(minerID, 120, 120)

	// Setup Migration Handler on System 1
	_, _ = RegisterMigrationHandler(bus, world1, grid1, sys1ID, logger)

	// 3. Setup System 2 (Target)
	sys2ID := uint32(2)
	world2 := ecs.NewWorld()
	grid2 := spatial.NewHashGrid(100)
	jgSys2 := NewJumpGateSystem(bus, sys2ID, logger)
	aiSys2 := NewAISystem(500.0, 10, 10000, 10000)
	moveSys2 := NewMovementSystem(10000, 10000)

	// Seeding System 2: Station and Jump Gate (no asteroids)
	station := world2.CreateEntity(domain.EntityStation)
	world2.AddComponent(station, &domain.Transform{X: -300, Y: -300})
	world2.AddComponent(station, &domain.StationMarket{
		Items: map[domain.ResourceType]*domain.MarketItem{
			domain.ResourceIron: {BasePrice: 10},
		},
	})
	grid2.Insert(station, -300, -300)

	gate2 := world2.CreateEntity(domain.EntityJumpGate)
	world2.AddComponent(gate2, &domain.Transform{X: -2000, Y: -2000})
	world2.AddComponent(gate2, &domain.JumpGate{
		TargetSystemID: 1,
		TargetX:        1800,
		TargetY:        1800,
	})
	grid2.Insert(gate2, -2000, -2000)

	// Setup Migration Handler on System 2
	_, _ = RegisterMigrationHandler(bus, world2, grid2, sys2ID, logger)

	// --- SIMULATION STEP 1: Miner Mines and Fills Cargo in System 1 ---
	// Miner finds asteroid and starts mining
	aiSys1.Update(world1, 0.05)
	cVal, _ := world1.GetComponent(minerID, domain.Cargo{})
	cargo := cVal.(*domain.Cargo)

	// Cheat: fill cargo capacity to trigger docking state
	cargo.Items = []domain.ItemInstance{
		{DefinitionID: 1, Quantity: cargo.Capacity, State: "normal"},
	}

	// Next AI update changes behavior to BehaviorDock
	aiSys1.Update(world1, 0.05)
	aiVal, _ := world1.GetComponent(minerID, domain.AIState{})
	ai := aiVal.(*domain.AIState)
	if ai.Behavior != domain.BehaviorDock {
		t.Fatalf("Expected behavior to be BehaviorDock, got %s", ai.Behavior)
	}

	// Since there is no station in System 1, the miner should set velocity towards Jump Gate (2000, 2000)
	aiSys1.Update(world1, 0.05)
	vVal, _ := world1.GetComponent(minerID, domain.Velocity{})
	vel := vVal.(*domain.Velocity)
	if vel.X <= 0 || vel.Y <= 0 {
		t.Errorf("Expected positive velocity towards gate, got (%.1f, %.1f)", vel.X, vel.Y)
	}

	// Move miner directly to Jump Gate to trigger migration
	tVal, _ := world1.GetComponent(minerID, domain.Transform{})
	trans := tVal.(*domain.Transform)
	trans.X = 1990
	trans.Y = 1990

	// Run systems update on System 1
	moveSys1.Update(world1, 0.05)
	jgSys1.Update(world1, 0.05)
	time.Sleep(50 * time.Millisecond) // wait NATS propagation

	// Verify migrated to System 2
	_, foundInSys1 := world1.GetEntityType(minerID)
	_, foundInSys2 := world2.GetEntityType(minerID)
	if foundInSys1 || !foundInSys2 {
		t.Fatalf("Expected miner to be migrated to System 2. Found in System 1: %t, Found in System 2: %t", foundInSys1, foundInSys2)
	}

	// --- SIMULATION STEP 2: Miner Sells Cargo in System 2 ---
	// On System 2, the miner is at (-1800, -1800) in BehaviorDock state.
	// Running AI system on System 2 will make it find the station at (-300, -300) and steer towards it.
	aiSys2.Update(world2, 0.05)
	vVal2, _ := world2.GetComponent(minerID, domain.Velocity{})
	vel2 := vVal2.(*domain.Velocity)
	if vel2.X <= 0 || vel2.Y <= 0 {
		t.Errorf("Expected velocity towards station (-300, -300), got (%.1f, %.1f)", vel2.X, vel2.Y)
	}

	// Teleport miner directly to station to sell cargo
	tVal2, _ := world2.GetComponent(minerID, domain.Transform{})
	trans2 := tVal2.(*domain.Transform)
	trans2.X = -300
	trans2.Y = -300

	// Next AI update sells cargo and switches to BehaviorIdle
	aiSys2.Update(world2, 0.05)
	cVal2, _ := world2.GetComponent(minerID, domain.Cargo{})
	cargo2 := cVal2.(*domain.Cargo)
	if len(cargo2.Items) != 0 {
		t.Error("Expected cargo to be empty after selling at station")
	}

	aiVal2, _ := world2.GetComponent(minerID, domain.AIState{})
	ai2 := aiVal2.(*domain.AIState)
	if ai2.Behavior != domain.BehaviorIdle {
		t.Fatalf("Expected behavior to return to BehaviorIdle, got %s", ai2.Behavior)
	}

	// --- SIMULATION STEP 3: Miner Returns to System 1 to Mine ---
	// Miner wants to mine, but there are no asteroids in System 2.
	// Running AI update should make it steer towards System 2's Jump Gate (-2000, -2000).
	aiSys2.Update(world2, 0.05)
	if vel2.X >= 0 || vel2.Y >= 0 {
		t.Errorf("Expected negative velocity towards return gate (-2000, -2000), got (%.1f, %.1f)", vel2.X, vel2.Y)
	}

	// Teleport miner directly to System 2's Jump Gate
	trans2.X = -1990
	trans2.Y = -1990

	// Run systems update on System 2 to trigger migration back to System 1
	moveSys2.Update(world2, 0.05)
	jgSys2.Update(world2, 0.05)
	time.Sleep(50 * time.Millisecond) // wait NATS propagation

	// Verify migrated back to System 1
	_, foundInSys1Again := world1.GetEntityType(minerID)
	_, foundInSys2Again := world2.GetEntityType(minerID)
	if !foundInSys1Again || foundInSys2Again {
		t.Fatalf("Expected miner to return to System 1. Found in System 1: %t, Found in System 2: %t", foundInSys1Again, foundInSys2Again)
	}

	t.Log("Cyclic cross-system miner trade route simulation passed successfully!")
}

