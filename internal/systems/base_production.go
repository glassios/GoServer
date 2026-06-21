package systems

import (
	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// BaseProductionSystem (Phase 5) makes space bases productive: every interval each base credits its
// owner's cargo with refined materials scaled by the base level. Credit only lands if the owner is
// present in this system and has free cargo space, so production is "collected" simply by owning a
// base near where you operate.
type BaseProductionSystem struct {
	interval float64
	acc      float64
}

func NewBaseProductionSystem() *BaseProductionSystem {
	return &BaseProductionSystem{interval: 5.0}
}

func (s *BaseProductionSystem) Name() string  { return "BaseProductionSystem" }
func (s *BaseProductionSystem) Priority() int { return 84 }

// baseProductionPerLevel is the IronPlates produced per base level per interval.
const baseProductionPerLevel = 2

func (s *BaseProductionSystem) Update(world *ecs.World, dt float64) {
	bases := world.Query(ecs.BuildMask(domain.SpaceBase{}))
	if len(bases) == 0 {
		return
	}
	s.acc += dt
	if s.acc < s.interval {
		return
	}
	s.acc = 0

	for _, baseID := range bases {
		bVal, _ := world.GetComponent(baseID, domain.SpaceBase{})
		base := bVal.(*domain.SpaceBase)

		ownerID := domain.EntityID(base.OwnerID)
		cargoVal, ok := world.GetComponent(ownerID, domain.Cargo{})
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
}
