package systems

import (
	"math"
	"time"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

type CombatSystem struct {
	eventBus        domain.EventBus
	accumulatedTime float64
}

func NewCombatSystem(eventBus domain.EventBus) *CombatSystem {
	return &CombatSystem{
		eventBus: eventBus,
	}
}

func (s *CombatSystem) Name() string {
	return "CombatSystem"
}

func (s *CombatSystem) Priority() int {
	return 80 // Executed after movement
}

func (s *CombatSystem) Update(world *ecs.World, dt float64) {
	s.accumulatedTime += dt

	mask := ecs.BuildMask(domain.Transform{}, domain.Weapon{})
	entities := world.Query(mask)

	// Reset IsFiring for all weapons
	for _, attackerID := range entities {
		if wVal, ok := world.GetComponent(attackerID, domain.Weapon{}); ok {
			wVal.(*domain.Weapon).IsFiring = false
		}
	}

	for _, attackerID := range entities {
		wVal, foundW := world.GetComponent(attackerID, domain.Weapon{})
		tVal, foundT := world.GetComponent(attackerID, domain.Transform{})
		if !foundW || !foundT {
			continue
		}

		weapon := wVal.(*domain.Weapon)
		attackerTrans := tVal.(*domain.Transform)

		if !weapon.Active {
			continue
		}

		// Combat only allowed in instanced combat arenas where participants have a CombatTeam component.
		// On the main map, combat is disabled and fleets only approach/pursue without firing.
		if _, hasTeam := world.GetComponent(attackerID, domain.CombatTeam{}); !hasTeam {
			continue
		}

		// Check cooldown
		if s.accumulatedTime-weapon.LastFire < float64(weapon.Cooldown) {
			continue
		}

		targetID := weapon.TargetID
		// Check target existence
		targetType, exists := world.GetEntityType(targetID)
		if !exists {
			weapon.Active = false
			continue
		}

		// Get target components
		targetTVal, foundTargetT := world.GetComponent(targetID, domain.Transform{})
		targetHVal, foundTargetH := world.GetComponent(targetID, domain.Health{})
		if !foundTargetT || !foundTargetH {
			weapon.Active = false
			continue
		}

		targetTrans := targetTVal.(*domain.Transform)
		targetHealth := targetHVal.(*domain.Health)

		if targetHealth.Current <= 0 {
			weapon.Active = false
			continue
		}

		// Prevent friendly fire if teams are defined
		attackerTeamVal, attackerHasTeam := world.GetComponent(attackerID, domain.CombatTeam{})
		targetTeamVal, targetHasTeam := world.GetComponent(targetID, domain.CombatTeam{})
		if attackerHasTeam && targetHasTeam {
			attackerTeam := attackerTeamVal.(*domain.CombatTeam)
			targetTeam := targetTeamVal.(*domain.CombatTeam)
			if attackerTeam.TeamID == targetTeam.TeamID {
				weapon.Active = false
				continue
			}
		}

		// Range check
		dx := attackerTrans.X - targetTrans.X
		dy := attackerTrans.Y - targetTrans.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist > weapon.Range {
			continue // Out of range, keep active but don't shoot yet
		}

		// Perform Attack
		weapon.LastFire = s.accumulatedTime
		weapon.IsFiring = true

		damage := weapon.Damage
		shieldVal, foundShield := world.GetComponent(targetID, domain.Shield{})

		// Damage absorption
		if foundShield {
			shield := shieldVal.(*domain.Shield)
			if shield.Current > 0 {
				if shield.Current >= damage {
					shield.Current -= damage
					damage = 0
				} else {
					damage -= shield.Current
					shield.Current = 0
				}
			}
		}

		if damage > 0 {
			targetHealth.Current -= damage
			if targetHealth.Current < 0 {
				targetHealth.Current = 0
			}
		}

		isKilled := targetHealth.Current <= 0

		// Publish event
		if s.eventBus != nil {
			s.eventBus.Publish(domain.DamageDealtEvent{
				BaseEvent: domain.BaseEvent{
					Time: time.Now(),
				},
				AttackerID: attackerID,
				TargetID:   targetID,
				Damage:     weapon.Damage,
				IsKilled:   isKilled,
			})

			if isKilled {
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
