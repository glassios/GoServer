package systems

import (
	"math"
	"time"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/pkg/mathutil"
)

// CombatSystem implements the Phase 1 Starsector-style core combat sim:
//   - per-mount weapon groups (WeaponGroup) instead of a single weapon
//   - a flux economy (firing builds flux; overload drops shields & stops fire; venting clears it)
//   - layered damage (shield → armor → hull) with damage-type multipliers
//   - shield/flux dissipation and shield regeneration each tick
//
// The single Weapon component remains as the AI's targeting controller (AISystem sets
// Active/TargetID and uses Range for standoff); actual damage comes from WeaponGroup mounts.
type CombatSystem struct {
	eventBus domain.EventBus
}

func NewCombatSystem(eventBus domain.EventBus) *CombatSystem {
	return &CombatSystem{eventBus: eventBus}
}

func (s *CombatSystem) Name() string {
	return "CombatSystem"
}

func (s *CombatSystem) Priority() int {
	return 80 // Executed after movement/AI
}

func (s *CombatSystem) Update(world *ecs.World, dt float64) {
	dtf := float32(dt)

	// Pass 1: per-tick upkeep on every combat ship — flux dissipation/venting, shield up/down,
	// shield regen — and reset transient fire flags.
	for _, id := range world.Query(ecs.BuildMask(domain.FluxState{})) {
		s.upkeep(world, id, dtf)
	}
	for _, id := range world.Query(ecs.BuildMask(domain.CombatFx{})) {
		if fxVal, ok := world.GetComponent(id, domain.CombatFx{}); ok {
			fxVal.(*domain.CombatFx).ShotsFired = 0
		}
	}
	for _, id := range world.Query(ecs.BuildMask(domain.Weapon{})) {
		if wVal, ok := world.GetComponent(id, domain.Weapon{}); ok {
			wVal.(*domain.Weapon).IsFiring = false
		}
	}

	// Pass 2: firing — only ships with a weapon group inside a combat instance (CombatTeam).
	for _, attackerID := range world.Query(ecs.BuildMask(domain.Transform{}, domain.WeaponGroup{}, domain.CombatTeam{})) {
		s.fire(world, attackerID, dtf)
	}

	// Pass 3: role abilities (repair / support) on allies.
	for _, id := range world.Query(ecs.BuildMask(domain.CombatRole{}, domain.Transform{}, domain.CombatTeam{})) {
		s.roleAbility(world, id, dtf)
	}
}

// roleAbility applies the support/repair role abilities: repair restores a wounded ally's hull
// and armor; support bleeds off a flux-stressed ally's flux. The chosen ally is recorded as
// AssistTargetID so the visualizer can draw the assist beam.
func (s *CombatSystem) roleAbility(world *ecs.World, id domain.EntityID, dtf float32) {
	rVal, ok := world.GetComponent(id, domain.CombatRole{})
	if !ok {
		return
	}
	cr := rVal.(*domain.CombatRole)
	cr.AssistTargetID = 0
	if cr.Role != domain.RoleRepair && cr.Role != domain.RoleSupport {
		return
	}

	teamVal, _ := world.GetComponent(id, domain.CombatTeam{})
	myTeam := teamVal.(*domain.CombatTeam).TeamID
	tVal, _ := world.GetComponent(id, domain.Transform{})
	mt := tVal.(*domain.Transform)
	myPos := mathutil.NewVec2(mt.X, mt.Y)

	const assistRange = 700.0
	var bestAlly domain.EntityID
	bestMetric := float32(0)
	found := false

	for _, allyID := range world.Query(ecs.BuildMask(domain.Transform{}, domain.Health{}, domain.CombatTeam{})) {
		if allyID == id {
			continue
		}
		atVal, _ := world.GetComponent(allyID, domain.CombatTeam{})
		if atVal.(*domain.CombatTeam).TeamID != myTeam {
			continue
		}
		ahVal, _ := world.GetComponent(allyID, domain.Health{})
		ah := ahVal.(*domain.Health)
		if ah.Current <= 0 {
			continue
		}
		ptVal, _ := world.GetComponent(allyID, domain.Transform{})
		pt := ptVal.(*domain.Transform)
		if myPos.Distance(mathutil.NewVec2(pt.X, pt.Y)) > assistRange {
			continue
		}

		var metric float32
		if cr.Role == domain.RoleRepair {
			metric = float32(ah.Max - ah.Current)
			if arVal, ok := world.GetComponent(allyID, domain.ArmorGrid{}); ok {
				ar := arVal.(*domain.ArmorGrid)
				metric += ar.Max - ar.Current
			}
		} else { // support: help whoever is closest to flux overload
			if fVal, ok := world.GetComponent(allyID, domain.FluxState{}); ok {
				metric = fVal.(*domain.FluxState).Current
			}
		}
		if metric > bestMetric {
			bestMetric = metric
			bestAlly = allyID
			found = true
		}
	}

	if !found {
		return
	}
	cr.AssistTargetID = bestAlly

	if cr.Role == domain.RoleRepair {
		// Accumulate fractional hull repair (Health is int32).
		cr.AbilityTimer += 22 * float64(dtf)
		if hVal, ok := world.GetComponent(bestAlly, domain.Health{}); ok {
			h := hVal.(*domain.Health)
			if whole := int32(cr.AbilityTimer); whole > 0 && h.Current < h.Max {
				h.Current += whole
				cr.AbilityTimer -= float64(whole)
				if h.Current > h.Max {
					h.Current = h.Max
				}
			}
		}
		if arVal, ok := world.GetComponent(bestAlly, domain.ArmorGrid{}); ok {
			ar := arVal.(*domain.ArmorGrid)
			if ar.Current < ar.Max {
				ar.Current += 12 * dtf
				if ar.Current > ar.Max {
					ar.Current = ar.Max
				}
			}
		}
	} else { // support
		if fVal, ok := world.GetComponent(bestAlly, domain.FluxState{}); ok {
			f := fVal.(*domain.FluxState)
			f.Current -= 45 * dtf
			if f.Current < 0 {
				f.Current = 0
			}
		}
	}
}

// upkeep dissipates flux, manages overload/venting, drops/raises the shield and regenerates it.
func (s *CombatSystem) upkeep(world *ecs.World, id domain.EntityID, dtf float32) {
	fluxVal, ok := world.GetComponent(id, domain.FluxState{})
	if !ok {
		return
	}
	flux := fluxVal.(*domain.FluxState)

	// Once overloaded, force a vent until flux is fully cleared.
	if flux.Overloaded {
		flux.Venting = true
	}
	rate := flux.DissipationRate
	if flux.Venting {
		rate *= 3.0 // venting dumps flux quickly but the ship can't fire meanwhile
	}
	flux.Current -= rate * dtf
	if flux.Current <= 0 {
		flux.Current = 0
		flux.Overloaded = false
		flux.Venting = false
	}

	if sVal, ok := world.GetComponent(id, domain.Shield{}); ok {
		sh := sVal.(*domain.Shield)
		// Shield is down while overloaded or venting.
		sh.Down = flux.Overloaded || flux.Venting
		if !sh.Down && sh.Current < sh.Max && sh.RegenRate > 0 {
			sh.RegenAcc += sh.RegenRate * dtf
			if whole := int32(sh.RegenAcc); whole > 0 {
				sh.Current += whole
				sh.RegenAcc -= float32(whole)
				if sh.Current > sh.Max {
					sh.Current = sh.Max
					sh.RegenAcc = 0
				}
			}
		}
	}
}

// fire advances mount cooldowns and discharges every ready, in-range mount at the controller's target.
func (s *CombatSystem) fire(world *ecs.World, attackerID domain.EntityID, dtf float32) {
	wgVal, _ := world.GetComponent(attackerID, domain.WeaponGroup{})
	wg := wgVal.(*domain.WeaponGroup)

	// Advance mount cooldowns regardless of whether we end up firing.
	for i := range wg.Weapons {
		if wg.Weapons[i].Cooldown > 0 {
			wg.Weapons[i].Cooldown -= dtf
		}
	}

	// Targeting controller (set by AISystem).
	wVal, hasWeapon := world.GetComponent(attackerID, domain.Weapon{})
	if !hasWeapon {
		return
	}
	weapon := wVal.(*domain.Weapon)
	if !weapon.Active || weapon.TargetID == 0 {
		return
	}

	targetID := weapon.TargetID
	targetType, exists := world.GetEntityType(targetID)
	if !exists {
		weapon.Active = false
		return
	}
	targetTVal, okT := world.GetComponent(targetID, domain.Transform{})
	targetHVal, okH := world.GetComponent(targetID, domain.Health{})
	if !okT || !okH {
		weapon.Active = false
		return
	}
	targetHealth := targetHVal.(*domain.Health)
	if targetHealth.Current <= 0 {
		weapon.Active = false
		return
	}

	// No friendly fire within a team.
	if at, ok := world.GetComponent(attackerID, domain.CombatTeam{}); ok {
		if tt, ok2 := world.GetComponent(targetID, domain.CombatTeam{}); ok2 {
			if at.(*domain.CombatTeam).TeamID == tt.(*domain.CombatTeam).TeamID {
				weapon.Active = false
				return
			}
		}
	}

	// A ship that is overloaded or venting cannot fire.
	var aflux *domain.FluxState
	if fVal, ok := world.GetComponent(attackerID, domain.FluxState{}); ok {
		aflux = fVal.(*domain.FluxState)
		if aflux.Overloaded || aflux.Venting {
			return
		}
	}

	aTVal, _ := world.GetComponent(attackerID, domain.Transform{})
	atrans := aTVal.(*domain.Transform)
	ttrans := targetTVal.(*domain.Transform)
	dx := atrans.X - ttrans.X
	dy := atrans.Y - ttrans.Y
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	fired := 0
	for i := range wg.Weapons {
		m := &wg.Weapons[i]
		if m.Cooldown > 0 || dist > m.Definition.Range {
			continue
		}
		// Flux headroom check; running out triggers an overload.
		if aflux != nil && aflux.Current+m.Definition.FluxCost > aflux.Capacity {
			aflux.Current = aflux.Capacity
			aflux.Overloaded = true
			break
		}
		m.Cooldown = m.Definition.Cooldown
		if aflux != nil {
			aflux.Current += m.Definition.FluxCost
			if aflux.Current >= aflux.Capacity {
				aflux.Current = aflux.Capacity
				aflux.Overloaded = true
			}
		}
		fired++

		killed := s.applyDamage(world, targetID, m.Definition.DamagePerShot, m.Definition.DamageType)
		if s.eventBus != nil {
			s.eventBus.Publish(domain.DamageDealtEvent{
				BaseEvent:  domain.BaseEvent{Time: time.Now()},
				AttackerID: attackerID,
				TargetID:   targetID,
				Damage:     int32(m.Definition.DamagePerShot),
				IsKilled:   killed,
			})
		}
		if killed {
			if s.eventBus != nil {
				s.eventBus.Publish(domain.EntityDestroyedEvent{
					BaseEvent:  domain.BaseEvent{Time: time.Now()},
					EntityID:   targetID,
					EntityType: targetType,
				})
			}
			weapon.Active = false
			break
		}
	}

	if fired > 0 {
		weapon.IsFiring = true
		if fxVal, ok := world.GetComponent(attackerID, domain.CombatFx{}); ok {
			fxVal.(*domain.CombatFx).ShotsFired = int32(fired)
		}
	}
}

// applyDamage runs one shot through the shield → armor → hull layers with damage-type
// multipliers, raising the defender's flux for damage its shield absorbs. Returns true if the
// shot killed the target.
func (s *CombatSystem) applyDamage(world *ecs.World, targetID domain.EntityID, dmg float32, dtype string) bool {
	if dmg <= 0 {
		return false
	}
	if fxVal, ok := world.GetComponent(targetID, domain.CombatFx{}); ok {
		fxVal.(*domain.CombatFx).LastDamageType = dtype
	}

	remaining := dmg

	// 1. Shields (if raised and charged): absorb the shot and convert absorbed damage to flux.
	if sVal, ok := world.GetComponent(targetID, domain.Shield{}); ok {
		sh := sVal.(*domain.Shield)
		if !sh.Down && sh.Current > 0 {
			eff := sh.Efficiency
			if eff <= 0 {
				eff = 1.0
			}
			sd := remaining * domain.DamageMultiplier(dtype, domain.LayerShield)
			if sd <= float32(sh.Current) {
				sh.Current -= int32(sd)
				s.raiseFlux(world, targetID, sd*eff)
				return false // shot fully stopped by shields
			}
			// Shield breaks mid-shot: it absorbs a fraction, the rest bleeds through.
			absorbedFrac := float32(sh.Current) / sd
			s.raiseFlux(world, targetID, float32(sh.Current)*eff)
			sh.Current = 0
			remaining *= (1 - absorbedFrac)
		}
	}

	// 2. Armor.
	if remaining > 0 {
		if aVal, ok := world.GetComponent(targetID, domain.ArmorGrid{}); ok {
			ar := aVal.(*domain.ArmorGrid)
			if ar.Current > 0 {
				ad := remaining * domain.DamageMultiplier(dtype, domain.LayerArmor)
				if ad <= ar.Current {
					ar.Current -= ad
					return false
				}
				absorbedFrac := ar.Current / ad
				ar.Current = 0
				remaining *= (1 - absorbedFrac)
			}
		}
	}

	// 3. Hull.
	if remaining > 0 {
		if hVal, ok := world.GetComponent(targetID, domain.Health{}); ok {
			h := hVal.(*domain.Health)
			h.Current -= int32(remaining * domain.DamageMultiplier(dtype, domain.LayerHull))
			if h.Current <= 0 {
				h.Current = 0
				return true
			}
		}
	}
	return false
}

func (s *CombatSystem) raiseFlux(world *ecs.World, id domain.EntityID, amount float32) {
	if amount <= 0 {
		return
	}
	if fVal, ok := world.GetComponent(id, domain.FluxState{}); ok {
		flux := fVal.(*domain.FluxState)
		flux.Current += amount
		if flux.Current >= flux.Capacity {
			flux.Current = flux.Capacity
			flux.Overloaded = true
		}
	}
}
