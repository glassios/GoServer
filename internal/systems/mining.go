package systems

import (
	"math"
	"time"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
)

type MiningSystem struct {
	eventBus        domain.EventBus
	progressBus     messaging.MessageBus // optional: pushes S_PLAYER_PROGRESS on XP gain
	accumulatedTime float64
}

func NewMiningSystem(eventBus domain.EventBus) *MiningSystem {
	return &MiningSystem{
		eventBus: eventBus,
	}
}

// SetProgressBus wires the messaging bus used to push skill/XP updates to mining players.
func (s *MiningSystem) SetProgressBus(bus messaging.MessageBus) {
	s.progressBus = bus
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

		// Calculate mine amount. The mining skill scales yield (Phase 3).
		var progress *domain.PlayerProgress
		if pVal, hasP := world.GetComponent(minerID, domain.PlayerProgress{}); hasP {
			progress = pVal.(*domain.PlayerProgress)
		}

		mineAmount := int32(laser.Power)
		if progress != nil {
			mineAmount = int32(float32(mineAmount) * progress.MiningYieldMult())
		}
		if mineAmount < int32(laser.Power) {
			mineAmount = int32(laser.Power) // never below base
		}
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

		// Award mining XP and push progress to the player.
		if progress != nil {
			progress.AddXP(domain.SkillMining, mineAmount)
			PublishPlayerProgress(s.progressBus, world, minerID)
		}

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
