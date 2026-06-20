package systems

import (
	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/spatial"
)

type VisibilitySystem struct {
	grid *spatial.HashGrid
}

func NewVisibilitySystem(grid *spatial.HashGrid) *VisibilitySystem {
	return &VisibilitySystem{
		grid: grid,
	}
}

func (s *VisibilitySystem) Name() string {
	return "VisibilitySystem"
}

func (s *VisibilitySystem) Priority() int {
	return 10 // Runs near the end of the tick, after all movements
}

func (s *VisibilitySystem) Update(world *ecs.World, dt float64) {
	// 1. Update positions of ALL entities with Transform in the Spatial Hash Grid
	tMask := ecs.BuildMask(domain.Transform{})
	tEntities := world.Query(tMask)

	for _, id := range tEntities {
		tVal, _ := world.GetComponent(id, domain.Transform{})
		trans := tVal.(*domain.Transform)
		s.grid.Update(id, trans.X, trans.Y)
	}

	// Clean up grid from entities that were destroyed (not queried by Transform anymore)
	// We can let the CleanupSystem trigger grid.Remove(id) or handle it inside CleanupSystem.
	// But to keep it simple, if an entity is destroyed, CleanupSystem will destroy it in world,
	// and we can handle removal from grid in a separate hook, or we can just clean the grid.
	// Let's add grid.Remove(id) to CleanupSystem, or handle it here by checking active entities.
	// However, because we rebuild the grid links on Update, active entities are kept updated.
	// A cleaner way is to let the CleanupSystem call grid.Remove(id) or we can do it on EntityDestroyedEvent.
	// For now, updating active ones covers all valid ones.

	// 2. Query visible entities for players/observers
	vMask := ecs.BuildMask(domain.Transform{}, domain.Visibility{})
	vEntities := world.Query(vMask)

	for _, observerID := range vEntities {
		tVal, _ := world.GetComponent(observerID, domain.Transform{})
		visVal, _ := world.GetComponent(observerID, domain.Visibility{})

		trans := tVal.(*domain.Transform)
		vis := visVal.(*domain.Visibility)

		// Get broadphase candidates from grid
		candidates := s.grid.QueryRadius(trans.X, trans.Y, vis.Radius)

		// Clear previous visible entities
		vis.VisibleEntities = make(map[domain.EntityID]struct{})

		radSq := vis.Radius * vis.Radius

		for _, candID := range candidates {
			if candID == observerID {
				continue // Don't add myself
			}

			candTVal, found := world.GetComponent(candID, domain.Transform{})
			if !found {
				continue
			}

			candTrans := candTVal.(*domain.Transform)
			dx := trans.X - candTrans.X
			dy := trans.Y - candTrans.Y
			distSq := dx*dx + dy*dy

			if distSq <= radSq {
				vis.VisibleEntities[candID] = struct{}{}
			}
		}
	}
}
