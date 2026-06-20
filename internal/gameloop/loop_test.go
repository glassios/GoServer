package gameloop

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

type TestSystem struct {
	updateCount int
	priority    int
}

func (s *TestSystem) Name() string { return "TestSystem" }
func (s *TestSystem) Priority() int { return s.priority }
func (s *TestSystem) Update(world *ecs.World, dt float64) {
	s.updateCount++
	mask := ecs.BuildMask(domain.Transform{}, domain.Velocity{})
	entities := world.Query(mask)
	for _, id := range entities {
		tVal, _ := world.GetComponent(id, domain.Transform{})
		vVal, _ := world.GetComponent(id, domain.Velocity{})
		trans := tVal.(*domain.Transform)
		vel := vVal.(*domain.Velocity)
		trans.X += vel.X * float32(dt)
		trans.Y += vel.Y * float32(dt)
	}
}

func TestGameLoop_LifecycleAndCommands(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	world := ecs.NewWorld()

	// Add dummy player entity
	player := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(player, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(player, &domain.Velocity{X: 0, Y: 0})

	sys := &TestSystem{priority: 10}
	loop := NewGameLoop(world, []ecs.System{sys}, 100, logger) // 100 TPS for faster test

	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)

	// Wait for loop to start and run a few ticks
	time.Sleep(50 * time.Millisecond)

	// Send command to set player velocity
	loop.EnqueueCommand(Command{
		PlayerID: player,
		Type:     "move",
		Payload:  domain.Velocity{X: 100, Y: 200},
	})

	// Wait some more ticks
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for loop to fully stop
	time.Sleep(20 * time.Millisecond)

	if loop.IsRunning() {
		t.Error("expected game loop to stop running")
	}

	if sys.updateCount == 0 {
		t.Error("expected system updates to occur")
	}

	// Verify command application
	vVal, _ := world.GetComponent(player, domain.Velocity{})
	vel := vVal.(*domain.Velocity)
	if vel.X != 100 || vel.Y != 200 {
		t.Errorf("expected velocity (100, 200) to be applied, got (%f, %f)", vel.X, vel.Y)
	}

	tTransVal, _ := world.GetComponent(player, domain.Transform{})
	trans := tTransVal.(*domain.Transform)
	if trans.X == 0 || trans.Y == 0 {
		t.Errorf("expected player entity to move, got (%f, %f)", trans.X, trans.Y)
	}
}

func TestScheduler_Sorting(t *testing.T) {
	sys1 := &TestSystem{priority: 5}
	sys2 := &TestSystem{priority: 50}
	sys3 := &TestSystem{priority: 25}

	sched := NewScheduler([]ecs.System{sys1, sys2, sys3})
	ordered := sched.GetSystems()

	if ordered[0].Priority() != 50 {
		t.Errorf("expected highest priority 50 first, got %d", ordered[0].Priority())
	}
	if ordered[1].Priority() != 25 {
		t.Errorf("expected medium priority 25 second, got %d", ordered[1].Priority())
	}
	if ordered[2].Priority() != 5 {
		t.Errorf("expected lowest priority 5 third, got %d", ordered[2].Priority())
	}
}
