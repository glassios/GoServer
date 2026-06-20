package gameloop

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

type Command struct {
	PlayerID domain.EntityID
	Type     string
	Payload  interface{}
}

type GameLoop struct {
	world          *ecs.World
	scheduler      *Scheduler
	tickRate       int           // e.g., 20
	tickDuration   time.Duration // e.g., 50ms
	commandQueue   chan Command
	logger         *zap.Logger
	metrics        *TickMetrics
	isRunning      bool
	runningMutex   sync.RWMutex
	accumulatedTime float64
	
	// Hook for snapshot generation (injected from outside or default)
	OnSnapshot func(tick uint64)
}

func NewGameLoop(world *ecs.World, systems []ecs.System, tickRate int, logger *zap.Logger) *GameLoop {
	tickDuration := time.Second / time.Duration(tickRate)
	return &GameLoop{
		world:        world,
		scheduler:    NewScheduler(systems),
		tickRate:     tickRate,
		tickDuration: tickDuration,
		commandQueue: make(chan Command, 10000),
		logger:       logger,
		metrics:      NewTickMetrics(100),
		OnSnapshot:   func(tick uint64) {},
	}
}

func (g *GameLoop) EnqueueCommand(cmd Command) {
	g.commandQueue <- cmd
}

func (g *GameLoop) Run(ctx context.Context) {
	g.runningMutex.Lock()
	if g.isRunning {
		g.runningMutex.Unlock()
		return
	}
	g.isRunning = true
	g.runningMutex.Unlock()

	g.logger.Info("Game loop started", zap.Int("tickRate", g.tickRate), zap.Duration("tickDuration", g.tickDuration))

	ticker := time.NewTicker(g.tickDuration)
	defer ticker.Stop()

	var tickCount uint64
	dt := g.tickDuration.Seconds()

	for {
		select {
		case <-ctx.Done():
			g.logger.Info("Game loop stopping via context cancellation...")
			g.runningMutex.Lock()
			g.isRunning = false
			g.runningMutex.Unlock()
			return
		case <-ticker.C:
			start := time.Now()
			tickCount++

			g.processCommands()
			g.update(dt)
			g.generateSnapshots(tickCount)

			duration := time.Since(start)
			g.metrics.Record(duration)

			if tickCount%200 == 0 { // Every 10 seconds at 20 TPS
				g.logger.Info("Tick engine status",
					zap.Uint64("ticks", tickCount),
					zap.Duration("avgDuration", g.metrics.Average()),
					zap.Duration("p99Duration", g.metrics.P99()),
					zap.Int("entities", g.world.EntityCount()),
				)
			}

			// Warning if tick took longer than allocated time
			if duration > g.tickDuration {
				g.logger.Warn("Tick took longer than allocated interval!",
					zap.Uint64("tick", tickCount),
					zap.Duration("duration", duration),
					zap.Duration("allocated", g.tickDuration),
				)
			}
		}
	}
}

func (g *GameLoop) processCommands() {
	// Drain all commands currently in the queue for this tick
	qLen := len(g.commandQueue)
	for i := 0; i < qLen; i++ {
		select {
		case cmd := <-g.commandQueue:
			g.handleCommand(cmd)
		default:
			return
		}
	}
}

func (g *GameLoop) handleCommand(cmd Command) {
	// Execute player intents by modifying components
	switch cmd.Type {
	case "move":
		if vel, ok := cmd.Payload.(domain.Velocity); ok {
			vVal, found := g.world.GetComponent(cmd.PlayerID, domain.Velocity{})
			if found {
				v := vVal.(*domain.Velocity)
				v.X = vel.X
				v.Y = vel.Y
			}
			// Если игрок делает ручное движение на карте, отменяем преследование/атаку
			wVal, foundW := g.world.GetComponent(cmd.PlayerID, domain.Weapon{})
			if foundW {
				weapon := wVal.(*domain.Weapon)
				weapon.Active = false
				weapon.TargetID = 0
			}
		}
	case "shoot":
		if payload, ok := cmd.Payload.(struct {
			Active   bool
			TargetID domain.EntityID
		}); ok {
			wVal, found := g.world.GetComponent(cmd.PlayerID, domain.Weapon{})
			if found {
				weapon := wVal.(*domain.Weapon)
				weapon.Active = payload.Active
				weapon.TargetID = payload.TargetID
			}
		}
	case "mine":
		if payload, ok := cmd.Payload.(struct {
			Active   bool
			TargetID domain.EntityID
		}); ok {
			lVal, found := g.world.GetComponent(cmd.PlayerID, domain.MiningLaser{})
			if found {
				laser := lVal.(*domain.MiningLaser)
				laser.Active = payload.Active
				laser.TargetID = payload.TargetID
			}
		}
	}
}

func (g *GameLoop) update(dt float64) {
	g.accumulatedTime += dt
	for _, sys := range g.scheduler.GetSystems() {
		sys.Update(g.world, dt)
	}
}

func (g *GameLoop) generateSnapshots(tick uint64) {
	if g.OnSnapshot != nil {
		g.OnSnapshot(tick)
	}
}

func (g *GameLoop) IsRunning() bool {
	g.runningMutex.RLock()
	defer g.runningMutex.RUnlock()
	return g.isRunning
}

func (g *GameLoop) Stop() {
	g.runningMutex.Lock()
	g.isRunning = false
	g.runningMutex.Unlock()
}

func (g *GameLoop) World() *ecs.World {
	return g.world
}
