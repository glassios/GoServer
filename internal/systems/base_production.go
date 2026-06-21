package systems

import (
	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// BaseProductionSystem (Phase 5) makes structures productive every interval: space bases refine
// IronPlates into their owner's cargo (scaled by base level), and developed planets pay their owner
// passive credits (scaled by development level). Output lands only if the owner is present in this
// system, so production is "collected" simply by operating where you own structures.
type BaseProductionSystem struct {
	interval float64
	acc      float64
}

func NewBaseProductionSystem() *BaseProductionSystem {
	return &BaseProductionSystem{interval: 5.0}
}

func (s *BaseProductionSystem) Name() string  { return "BaseProductionSystem" }
func (s *BaseProductionSystem) Priority() int { return 84 }

// Per-level yields per interval.
const (
	baseProductionPerLevel = 2  // IronPlates per base level
	planetCreditsPerLevel  = 25 // credits per planet development level
)

func (s *BaseProductionSystem) Update(world *ecs.World, dt float64) {
	s.acc += dt
	if s.acc < s.interval {
		return
	}
	s.acc = 0

	// Space bases -> owner cargo (IronPlates).
	for _, baseID := range world.Query(ecs.BuildMask(domain.SpaceBase{})) {
		bVal, _ := world.GetComponent(baseID, domain.SpaceBase{})
		base := bVal.(*domain.SpaceBase)

		cargoVal, ok := world.GetComponent(domain.EntityID(base.OwnerID), domain.Cargo{})
		if !ok {
			continue // owner offline / in another system
		}
		cargo := cargoVal.(*domain.Cargo)

		var load int32
		for _, it := range cargo.Items {
			load += it.Quantity
		}
		free := cargo.Capacity - load
		if free <= 0 {
			continue
		}
		amount := base.Level * baseProductionPerLevel
		if amount > free {
			amount = free
		}
		if amount > 0 {
			cargo.AddResourceTypeQuantity("IronPlates", amount)
		}
	}

	// Planets -> owner credits.
	for _, planetID := range world.Query(ecs.BuildMask(domain.Planet{})) {
		pVal, _ := world.GetComponent(planetID, domain.Planet{})
		planet := pVal.(*domain.Planet)
		if planet.OwnerID == 0 || planet.DevelopmentLevel <= 0 {
			continue
		}
		if ownerVal, ok := world.GetComponent(domain.EntityID(planet.OwnerID), domain.PlayerData{}); ok {
			ownerVal.(*domain.PlayerData).Credits += int64(planet.DevelopmentLevel) * planetCreditsPerLevel
		}
	}
}
