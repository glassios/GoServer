package systems

import (
	"sync"
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

type MockEventBus struct {
	mutex       sync.RWMutex
	subscribers map[string][]func(domain.Event)
	events      []domain.Event
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		subscribers: make(map[string][]func(domain.Event)),
		events:      make([]domain.Event, 0),
	}
}

func (b *MockEventBus) Publish(event domain.Event) {
	b.mutex.Lock()
	b.events = append(b.events, event)
	b.mutex.Unlock()

	b.mutex.RLock()
	handlers, exists := b.subscribers[event.EventType()]
	b.mutex.RUnlock()

	if exists {
		for _, handler := range handlers {
			handler(event)
		}
	}
}

func (b *MockEventBus) Subscribe(eventType string, handler func(domain.Event)) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
}

func TestSystems_Integration(t *testing.T) {
	world := ecs.NewWorld()
	bus := NewMockEventBus()

	// Initialize systems
	grid := spatial.NewHashGrid(200.0)
	moveSys := NewMovementSystem(10000, 10000)
	visSys := NewVisibilitySystem(grid)
	combatSys := NewCombatSystem(bus)
	miningSys := NewMiningSystem(bus)
	aiSys := NewAISystem(500.0, 2, 10000, 10000)
	cleanupSys := NewCleanupSystem(grid)

	// 1. Create a Miner NPC
	miner := world.CreateEntity(domain.EntityNPC)
	world.AddComponent(miner, &domain.Transform{X: 10, Y: 10})
	world.AddComponent(miner, &domain.Velocity{X: 0, Y: 0})
	world.AddComponent(miner, &domain.ShipConfig{ShipType: "miner", MaxSpeed: 50})
	world.AddComponent(miner, &domain.Cargo{Capacity: 100})
	world.AddComponent(miner, &domain.MiningLaser{Power: 10, Range: 50})
	world.AddComponent(miner, &domain.AIState{Behavior: domain.BehaviorIdle})
	world.AddComponent(miner, &domain.FactionMember{FactionID: 2}) // Miner faction

	// 2. Create an Asteroid
	asteroid := world.CreateEntity(domain.EntityAsteroid)
	world.AddComponent(asteroid, &domain.Transform{X: 30, Y: 30}) // very close to miner
	world.AddComponent(asteroid, &domain.AsteroidResource{Type: domain.ResourceIron, Amount: 30})

	// 3. Create a Pirate NPC
	pirate := world.CreateEntity(domain.EntityNPC)
	world.AddComponent(pirate, &domain.Transform{X: 100, Y: 100})
	world.AddComponent(pirate, &domain.Velocity{X: 0, Y: 0})
	world.AddComponent(pirate, &domain.ShipConfig{ShipType: "pirate", MaxSpeed: 60})
	world.AddComponent(pirate, &domain.Health{Current: 50, Max: 50})
	world.AddComponent(pirate, &domain.Weapon{Type: domain.WeaponLaser, Damage: 5, Range: 150, Cooldown: 1.0})
	world.AddComponent(pirate, &domain.AIState{Behavior: domain.BehaviorIdle, HomePos: domain.Transform{X: 100, Y: 100}})
	world.AddComponent(pirate, &domain.FactionMember{FactionID: 1}) // Pirate faction

	// 4. Create Player
	player := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(player, &domain.Transform{X: 120, Y: 120}) // close to pirate
	world.AddComponent(player, &domain.Velocity{X: 10, Y: 0})
	world.AddComponent(player, &domain.Health{Current: 100, Max: 100})
	world.AddComponent(player, &domain.Shield{Current: 50, Max: 50})
	world.AddComponent(player, &domain.ShipConfig{ShipType: "fighter", MaxSpeed: 80})
	world.AddComponent(player, &domain.Visibility{Radius: 500.0})

	// Run 100 ticks (dt = 0.05 seconds per tick)
	dt := 0.05
	for tick := 0; tick < 100; tick++ {
		aiSys.Update(world, dt)
		moveSys.Update(world, dt)
		visSys.Update(world, dt)
		combatSys.Update(world, dt)
		miningSys.Update(world, dt)
		cleanupSys.Update(world, dt)
	}

	// VERIFY 1: Movement applied to player
	pTransVal, _ := world.GetComponent(player, domain.Transform{})
	pTrans := pTransVal.(*domain.Transform)
	if pTrans.X <= 120 {
		t.Errorf("expected player to move, X coordinate got %f", pTrans.X)
	}

	// VERIFY 2: Miner NPC should have harvested resources
	cargoVal, _ := world.GetComponent(miner, domain.Cargo{})
	cargo := cargoVal.(*domain.Cargo)
	if cargo.GetResourceTypeQuantity(domain.ResourceIron) <= 0 {
		t.Errorf("expected miner NPC to gather iron, got %v", cargo.GetResourceTypeQuantity(domain.ResourceIron))
	}

	// VERIFY 3: Asteroid resource should be reduced or depleted
	astVal, foundAst := world.GetComponent(asteroid, domain.AsteroidResource{})
	if foundAst {
		ast := astVal.(*domain.AsteroidResource)
		if ast.Amount >= 30 {
			t.Errorf("expected asteroid resources to decrease, got %d", ast.Amount)
		}
	} else {
		// Asteroid was depleted and cleaned up, which is also correct
		t.Log("Asteroid fully depleted and cleaned up successfully")
	}

	// VERIFY 4: Combat is disabled, so player shield should NOT be damaged by pirate
	pShieldVal, _ := world.GetComponent(player, domain.Shield{})
	pShield := pShieldVal.(*domain.Shield)
	if pShield.Current < 50 {
		t.Errorf("expected player shield to remain undamaged, got %d", pShield.Current)
	}

	// VERIFY 5: Player visibility list should contain pirate (since miner is at (10,10) and player moved away)
	visVal, _ := world.GetComponent(player, domain.Visibility{})
	vis := visVal.(*domain.Visibility)
	if len(vis.VisibleEntities) == 0 {
		t.Error("expected player visibility list to contain nearby entities, but it is empty")
	}

	// Print Event list for inspection
	t.Logf("Total published events: %d", len(bus.events))
	for _, e := range bus.events {
		t.Logf("Event: %s at %v", e.EventType(), e.Timestamp())
	}
}

func BenchmarkSystems_Update_1000(b *testing.B) {
	world := ecs.NewWorld()
	bus := NewMockEventBus()

	grid := spatial.NewHashGrid(200.0)
	moveSys := NewMovementSystem(10000, 10000)
	visSys := NewVisibilitySystem(grid)
	combatSys := NewCombatSystem(bus)
	miningSys := NewMiningSystem(bus)
	aiSys := NewAISystem(500.0, 0, 10000, 10000)
	cleanupSys := NewCleanupSystem(grid)

	// Spawn 1000 entities
	for i := 0; i < 1000; i++ {
		var id ecs.ComponentMask // dummy
		_ = id
		if i%3 == 0 {
			// Player
			e := world.CreateEntity(domain.EntityPlayer)
			world.AddComponent(e, &domain.Transform{X: float32(i), Y: float32(i)})
			world.AddComponent(e, &domain.Velocity{X: 1, Y: 1})
			world.AddComponent(e, &domain.Health{Current: 100, Max: 100})
			world.AddComponent(e, &domain.Shield{Current: 50, Max: 50})
			world.AddComponent(e, &domain.ShipConfig{ShipType: "fighter", MaxSpeed: 50})
			world.AddComponent(e, &domain.Visibility{Radius: 500.0})
		} else if i%3 == 1 {
			// Asteroid
			e := world.CreateEntity(domain.EntityAsteroid)
			world.AddComponent(e, &domain.Transform{X: float32(i), Y: float32(i)})
			world.AddComponent(e, &domain.AsteroidResource{Type: domain.ResourceIron, Amount: 100})
		} else {
			// NPC
			e := world.CreateEntity(domain.EntityNPC)
			world.AddComponent(e, &domain.Transform{X: float32(i), Y: float32(i)})
			world.AddComponent(e, &domain.Velocity{X: 0, Y: 0})
			world.AddComponent(e, &domain.ShipConfig{ShipType: "miner", MaxSpeed: 40})
			world.AddComponent(e, &domain.Cargo{Capacity: 100})
			world.AddComponent(e, &domain.MiningLaser{Power: 5, Range: 50})
			world.AddComponent(e, &domain.AIState{Behavior: domain.BehaviorIdle, HomePos: domain.Transform{X: float32(i), Y: float32(i)}})
			world.AddComponent(e, &domain.FactionMember{FactionID: 2})
		}
	}

	dt := 0.05
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aiSys.Update(world, dt)
		moveSys.Update(world, dt)
		visSys.Update(world, dt)
		combatSys.Update(world, dt)
		miningSys.Update(world, dt)
		cleanupSys.Update(world, dt)
	}
}
