package systems

import (
	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

type ShipyardSystem struct{}

func NewShipyardSystem() *ShipyardSystem {
	return &ShipyardSystem{}
}

func (s *ShipyardSystem) Name() string {
	return "ShipyardSystem"
}

func (s *ShipyardSystem) Priority() int {
	return 81
}

func (s *ShipyardSystem) Update(world *ecs.World, dt float64) {
	mask := ecs.BuildMask(domain.Shipyard{})
	entities := world.Query(mask)

	for _, stationID := range entities {
		syVal, ok := world.GetComponent(stationID, domain.Shipyard{})
		if !ok {
			continue
		}

		sy := syVal.(*domain.Shipyard)
		if len(sy.Queue) == 0 {
			continue
		}

		order := &sy.Queue[0]
		order.Progress += float32(dt)

		if order.Progress >= order.TotalTime {
			// Order finished! Deliver the ship
			playerID := domain.EntityID(order.OrderedBy)
			if _, exists := world.GetEntityType(playerID); exists {
				if cfgVal, ok := world.GetComponent(playerID, domain.ShipConfig{}); ok {
					cfg := cfgVal.(*domain.ShipConfig)
					cfg.ShipType = order.ShipType
					if order.ShipType == "fighter" {
						cfg.MaxSpeed = 120
						cfg.TurnRate = 1.8
					} else if order.ShipType == "miner" {
						cfg.MaxSpeed = 60
						cfg.TurnRate = 1.0
					}

					// Reset Health and Shield to full on new ship delivery
					if hVal, ok := world.GetComponent(playerID, domain.Health{}); ok {
						h := hVal.(*domain.Health)
						h.Current = h.Max
					}
					if sVal, ok := world.GetComponent(playerID, domain.Shield{}); ok {
						s := sVal.(*domain.Shield)
						s.Current = s.Max
					}
				}
			}

			// Dequeue order
			sy.Queue = sy.Queue[1:]
		}
	}
}
