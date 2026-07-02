package gameloop

import (
	"context"
	"sort"
	"strings"
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

	// Per-system timing of the most recent update(), reused each tick (no per-tick alloc).
	sysTimings []systemTiming

	// Hook for snapshot generation (injected from outside or default)
	OnSnapshot func(tick uint64)
}

type systemTiming struct {
	name string
	dur  time.Duration
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

			tCmd := time.Now()
			g.processCommands()
			cmdDur := time.Since(tCmd)

			tUpd := time.Now()
			g.update(dt)
			updDur := time.Since(tUpd)

			tSnap := time.Now()
			g.generateSnapshots(tickCount)
			snapDur := time.Since(tSnap)

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

			// Warning if tick took longer than allocated time — log the phase + per-system
			// breakdown so the offender is identifiable instead of guessed.
			if duration > g.tickDuration {
				g.logger.Warn("Tick took longer than allocated interval!",
					zap.Uint64("tick", tickCount),
					zap.Duration("duration", duration),
					zap.Duration("allocated", g.tickDuration),
					zap.Int("entities", g.world.EntityCount()),
					zap.Duration("commands", cmdDur),
					zap.Duration("update", updDur),
					zap.Duration("snapshot", snapDur),
					zap.String("systems", g.lastSystemTimings()),
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
			// Ручное движение на карте отменяет все текущие приказы: атаку, добычу и эскорт.
			wVal, foundW := g.world.GetComponent(cmd.PlayerID, domain.Weapon{})
			if foundW {
				weapon := wVal.(*domain.Weapon)
				weapon.Active = false
				weapon.TargetID = 0
			}
			if lVal, foundL := g.world.GetComponent(cmd.PlayerID, domain.MiningLaser{}); foundL {
				laser := lVal.(*domain.MiningLaser)
				laser.Active = false
				laser.TargetID = 0
			}
			if _, hasFollow := g.world.GetComponent(cmd.PlayerID, domain.FollowOrder{}); hasFollow {
				g.world.RemoveComponent(cmd.PlayerID, domain.FollowOrder{})
			}
			if _, hasPending := g.world.GetComponent(cmd.PlayerID, domain.PendingJoin{}); hasPending {
				g.world.RemoveComponent(cmd.PlayerID, domain.PendingJoin{})
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
			// Открытие огня отменяет добычу и эскорт.
			if payload.Active {
				if lVal, foundL := g.world.GetComponent(cmd.PlayerID, domain.MiningLaser{}); foundL {
					laser := lVal.(*domain.MiningLaser)
					laser.Active = false
					laser.TargetID = 0
				}
				if _, hasFollow := g.world.GetComponent(cmd.PlayerID, domain.FollowOrder{}); hasFollow {
					g.world.RemoveComponent(cmd.PlayerID, domain.FollowOrder{})
				}
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
			// Начало добычи отменяет атаку и эскорт.
			if payload.Active {
				if wVal, foundW := g.world.GetComponent(cmd.PlayerID, domain.Weapon{}); foundW {
					weapon := wVal.(*domain.Weapon)
					weapon.Active = false
					weapon.TargetID = 0
				}
				if _, hasFollow := g.world.GetComponent(cmd.PlayerID, domain.FollowOrder{}); hasFollow {
					g.world.RemoveComponent(cmd.PlayerID, domain.FollowOrder{})
				}
			}
		}
	}
}

func (g *GameLoop) update(dt float64) {
	g.accumulatedTime += dt
	systems := g.scheduler.GetSystems()
	g.sysTimings = g.sysTimings[:0]
	for _, sys := range systems {
		s := time.Now()
		sys.Update(g.world, dt)
		g.sysTimings = append(g.sysTimings, systemTiming{sys.Name(), time.Since(s)})
	}
}

// lastSystemTimings renders the slowest systems from the most recent update() as a
// compact "Name=1.2ms, ..." string (top 5), for the slow-tick warning log.
func (g *GameLoop) lastSystemTimings() string {
	t := make([]systemTiming, len(g.sysTimings))
	copy(t, g.sysTimings)
	sort.Slice(t, func(i, j int) bool { return t[i].dur > t[j].dur })

	var b strings.Builder
	for i, st := range t {
		if i >= 5 {
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(st.name)
		b.WriteByte('=')
		b.WriteString(st.dur.String())
	}
	return b.String()
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
