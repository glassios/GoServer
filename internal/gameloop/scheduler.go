package gameloop

import (
	"sort"

	"github.com/Home/galaxy-mmo/internal/ecs"
)

// Scheduler orders ECS systems according to their priority.
type Scheduler struct {
	systems []ecs.System
}

func NewScheduler(systems []ecs.System) *Scheduler {
	// Create a copy of the slice
	sysCopy := make([]ecs.System, len(systems))
	copy(sysCopy, systems)

	// Sort systems: higher priority first
	sort.Slice(sysCopy, func(i, j int) bool {
		return sysCopy[i].Priority() > sysCopy[j].Priority()
	})

	return &Scheduler{
		systems: sysCopy,
	}
}

func (s *Scheduler) GetSystems() []ecs.System {
	return s.systems
}
