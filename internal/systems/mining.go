package systems

import (
	"math"
	"time"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

type MiningSystem struct {
	eventBus        domain.EventBus
	accumulatedTime float64
}

func NewMiningSystem(eventBus domain.EventBus) *MiningSystem {
	return &MiningSystem{
		eventBus: eventBus,
	}
}

func (s *MiningSystem) Name() string {
	return "MiningSystem"
}

func (s *MiningSystem) Priority() int {
	return 70 // Runs after combat
}

func (s *MiningSystem) Update(world *ecs.World, dt float64) {
	s.accumulatedTime += dt

	mask := ecs.BuildMask(domain.Transform{}, domain.MiningLaser{}, domain.Cargo{})
	entities := world.Query(mask)

	for _, minerID := range entities {
		tVal, foundT := world.GetComponent(minerID, domain.Transform{})
		lVal, foundL := world.GetComponent(minerID, domain.MiningLaser{})
		cVal, foundC := world.GetComponent(minerID, domain.Cargo{})
		if !foundT || !foundL || !foundC {
			continue
		}

		minerTrans := tVal.(*domain.Transform)
		laser := lVal.(*domain.MiningLaser)
		cargo := cVal.(*domain.Cargo)

		if !laser.Active {
			continue
		}

		// Mine once per second
		if s.accumulatedTime-laser.LastMine < 1.0 {
			continue
		}

		targetID := laser.TargetID
		// Check target existence
		targetType, exists := world.GetEntityType(targetID)
		if !exists {
			laser.Active = false
			continue
		}

		// Target must be an asteroid and have AsteroidResource + Transform
		targetTVal, foundTargetT := world.GetComponent(targetID, domain.Transform{})
		resVal, foundRes := world.GetComponent(targetID, domain.AsteroidResource{})
		if !foundTargetT || !foundRes {
			laser.Active = false
			continue
		}

		targetTrans := targetTVal.(*domain.Transform)
		asteroid := resVal.(*domain.AsteroidResource)

		// Range check
		dx := minerTrans.X - targetTrans.X
		dy := minerTrans.Y - targetTrans.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist > laser.Range {
			continue // Out of range, wait
		}

		// Calculate cargo usage
		currentVolume := int32(0)
		for _, item := range cargo.Items {
			currentVolume += item.Quantity
		}

		freeSpace := cargo.Capacity - currentVolume
		if freeSpace <= 0 {
			laser.Active = false
			continue
		}

		// Calculate mine amount
		mineAmount := int32(laser.Power)
		if mineAmount > asteroid.Amount {
			mineAmount = asteroid.Amount
		}
		if mineAmount > freeSpace {
			mineAmount = freeSpace
		}

		if mineAmount <= 0 {
			laser.Active = false
			continue
		}

		// Perform Mining
		asteroid.Amount -= mineAmount
		cargo.AddResourceTypeQuantity(asteroid.Type, mineAmount)
		laser.LastMine = s.accumulatedTime

		// Fire Event
		if s.eventBus != nil {
			s.eventBus.Publish(domain.ResourceMinedEvent{
				BaseEvent: domain.BaseEvent{
					Time: time.Now(),
				},
				MinerID:    minerID,
				AsteroidID: targetID,
				Resource:   asteroid.Type,
				Amount:     mineAmount,
			})
		}

		// Asteroid depleted
		if asteroid.Amount <= 0 {
			laser.Active = false
			// Trigger depletion / destruction event
			if s.eventBus != nil {
				s.eventBus.Publish(domain.EntityDestroyedEvent{
					BaseEvent: domain.BaseEvent{
						Time: time.Now(),
					},
					EntityID:   targetID,
					EntityType: targetType,
				})
			}
		}
	}
}
