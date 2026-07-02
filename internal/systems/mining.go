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
	// mined accumulates miner IDs whose cargo grew this tick, so the world node can
	// push an s_inventory_update for them (mining has no client command/response).
	mined map[domain.EntityID]bool
}

func NewMiningSystem(eventBus domain.EventBus) *MiningSystem {
	return &MiningSystem{
		eventBus: eventBus,
		mined:    make(map[domain.EntityID]bool),
	}
}

// DrainUpdated returns and clears the set of miner IDs whose cargo changed since the
// last call (so the caller can send each an inventory update).
func (s *MiningSystem) DrainUpdated() []domain.EntityID {
	if len(s.mined) == 0 {
		return nil
	}
	ids := make([]domain.EntityID, 0, len(s.mined))
	for id := range s.mined {
		ids = append(ids, id)
	}
	s.mined = make(map[domain.EntityID]bool)
	return ids
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

		// Cargo capacity is measured in VOLUME units; convert free volume to a unit count
		// for the mined resource.
		unitVol := domain.VolumeForID(domain.ResourceToID[asteroid.Type])
		if unitVol <= 0 {
			unitVol = 1
		}
		freeVol := float32(cargo.Capacity) - cargo.LoadVolume()
		freeSpace := int32(freeVol / unitVol)
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
		s.mined[minerID] = true // flag for an inventory push this tick

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
