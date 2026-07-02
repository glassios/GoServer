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
	// Fires collects this tick's traveling-shot discharges (projectile-class weapons) so the
	// instance can stream them to the client as cosmetic fire events (thin channel, B3). Damage
	// is still applied instantly in fire(); this is purely presentational. Reset each Update.
	Fires []domain.FireEvent
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

	// Drop last tick's traveling-shot fire events; fire() refills them this tick.
	s.Fires = s.Fires[:0]

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
	// Decay missile subsystem disables (B4): engine/weapon hits wear off over a few seconds.
	for _, id := range world.Query(ecs.BuildMask(domain.SubsystemState{})) {
		ssVal, ok := world.GetComponent(id, domain.SubsystemState{})
		if !ok {
			continue
		}
		ss := ssVal.(*domain.SubsystemState)
		if ss.EngineHitTimer > 0 {
			ss.EngineHitTimer -= dtf
			if ss.EngineHitTimer < 0 {
				ss.EngineHitTimer = 0
			}
		}
		if ss.WeaponHitTimer > 0 {
			ss.WeaponHitTimer -= dtf
			if ss.WeaponHitTimer < 0 {
				ss.WeaponHitTimer = 0
			}
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

	// Overload is a fixed-duration lockout (Starsector-style): the ship vents fast, cannot
	// fire and drops its shield for OverloadTimer seconds regardless of flux level, then flux
	// resets to 0 and systems come back. This replaces the old "vent until empty" behaviour so
	// an overload is a real, observable window.
	if flux.Overloaded {
		flux.OverloadTimer -= dtf
		flux.Current -= flux.DissipationRate * 3.0 * dtf // fast vent while locked (cosmetic)
		if flux.Current < 0 {
			flux.Current = 0
		}
		if flux.OverloadTimer <= 0 {
			flux.Overloaded = false
			flux.Venting = false
			flux.OverloadTimer = 0
			flux.Current = 0
		} else {
			flux.Venting = true
		}
	} else {
		flux.Current -= flux.DissipationRate * dtf
		if flux.Current < 0 {
			flux.Current = 0
		}
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

	// Advance mount cooldowns + magazine reloads regardless of whether we end up firing.
	for i := range wg.Weapons {
		m := &wg.Weapons[i]
		if m.Cooldown > 0 {
			m.Cooldown -= dtf
		}
		if m.ReloadTimer > 0 {
			m.ReloadTimer -= dtf
			if m.ReloadTimer <= 0 {
				m.ReloadTimer = 0
				m.Ammo = m.Definition.Magazine // magazine refilled
			}
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

	// A ship whose weapons subsystem was knocked out by a missile cannot fire (B4).
	if ssVal, ok := world.GetComponent(attackerID, domain.SubsystemState{}); ok {
		if ssVal.(*domain.SubsystemState).WeaponHitTimer > 0 {
			return
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
		// Magazine weapons can't fire while reloading (B5).
		if m.ReloadTimer > 0 {
			continue
		}
		// Per-mount firing arc (B5): the target must lie within the mount's arc, measured from the
		// hull's facing plus the mount's rest angle. Wide turrets almost always bear; fixed
		// hardpoints only fire when the ship is pointed roughly at the target.
		if m.ArcHalf > 0 && m.ArcHalf < math.Pi {
			aim := atrans.Rotation + m.MountAngle
			bearing := float32(math.Atan2(float64(ttrans.Y-atrans.Y), float64(ttrans.X-atrans.X)))
			off := wrapPi(bearing - aim)
			if off < 0 {
				off = -off
			}
			if off > m.ArcHalf {
				continue
			}
		}
		// Flux headroom check; running out triggers an overload.
		if aflux != nil && aflux.Current+m.Definition.FluxCost > aflux.Capacity {
			triggerOverload(aflux)
			break
		}
		m.Cooldown = m.Definition.Cooldown
		if aflux != nil {
			aflux.Current += m.Definition.FluxCost
			if aflux.Current >= aflux.Capacity {
				triggerOverload(aflux)
			}
		}
		fired++

		// Magazine spend (B5): burst weapons deplete their magazine and then reload.
		if m.Definition.Magazine > 0 {
			m.Ammo--
			if m.Ammo <= 0 {
				m.ReloadTimer = m.Definition.ReloadTime
			}
		}

		// Barrels (B5): a mount discharges Barrels shots per trigger (volley). Default 1.
		barrels := m.Definition.Barrels
		if barrels < 1 {
			barrels = 1
		}
		for b := int32(0); b < barrels; b++ {
			// Firing cosmetic hint (B3, thin channel): damage is still applied instantly below, but a
			// classed mount (projectile/beam/missile) also emits a fire event so the client can draw the
			// shot — a flying bolt, an instant beam line, or a homing missile. Unclassed/hitscan mounts
			// emit nothing (the client falls back to its is_shooting line for those).
			if cls := m.Definition.Class; cls != "" && cls != domain.WeaponClassHitscan {
				s.Fires = append(s.Fires, domain.FireEvent{
					AttackerID:  attackerID,
					TargetID:    targetID,
					OriginX:     atrans.X,
					OriginY:     atrans.Y,
					TargetX:     ttrans.X,
					TargetY:     ttrans.Y,
					Speed:       m.Definition.ProjectileSpeed,
					DamageType:  m.Definition.DamageType,
					WeaponClass: m.Definition.Class,
				})
			}

			// Missiles are live entities (B4): spawn one that flies, homes and can be shot down by
			// point-defense; MissileSystem applies its damage on arrival. All other classes deal their
			// authoritative damage instantly here.
			if m.Definition.Class == domain.WeaponClassMissile {
				s.spawnMissile(world, attackerID, targetID, atrans, ttrans, &m.Definition)
				continue
			}

			killed := s.applyDamage(world, targetID, atrans.X, atrans.Y, m.Definition.DamagePerShot, m.Definition.DamageType, m.Definition.Class)
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
		if !weapon.Active {
			break // target killed → this ship stops firing its remaining mounts
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
func (s *CombatSystem) applyDamage(world *ecs.World, targetID domain.EntityID, attackerX, attackerY, dmg float32, dtype, weaponClass string) bool {
	if dmg <= 0 {
		return false
	}
	if fxVal, ok := world.GetComponent(targetID, domain.CombatFx{}); ok {
		fxVal.(*domain.CombatFx).LastDamageType = dtype
	}

	remaining := dmg

	// 1. Shields (if raised, charged, and covering the incoming direction): absorb the shot and
	//    convert absorbed damage to flux. A directional shield that doesn't face the shot lets it
	//    through to armor/hull.
	if sVal, ok := world.GetComponent(targetID, domain.Shield{}); ok {
		sh := sVal.(*domain.Shield)
		if !sh.Down && sh.Current > 0 && s.shieldBlocks(world, targetID, sh, attackerX, attackerY) {
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

	// A missile that gets past the shield (or hits an unshielded/exposed hull) knocks out a
	// subsystem for a few seconds (B4): first the engines, then the weapons on a later strike.
	if remaining > 0 && weaponClass == domain.WeaponClassMissile {
		s.applySubsystemHit(world, targetID)
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

// spawnMissile creates a live homing missile entity flying from the attacker toward the target
// (Phase B4). MissileSystem owns its flight/hit/interception; damage is dealt on arrival.
func (s *CombatSystem) spawnMissile(world *ecs.World, attackerID, targetID domain.EntityID, atrans, ttrans *domain.Transform, def *domain.WeaponDefinition) {
	angle := float32(math.Atan2(float64(ttrans.Y-atrans.Y), float64(ttrans.X-atrans.X)))
	var team uint32
	if ctVal, ok := world.GetComponent(attackerID, domain.CombatTeam{}); ok {
		team = ctVal.(*domain.CombatTeam).TeamID
	}
	speed := def.ProjectileSpeed
	if speed <= 0 {
		speed = 300
	}
	mid := world.CreateEntity(domain.EntityMissile)
	world.AddComponent(mid, &domain.Transform{X: atrans.X, Y: atrans.Y, Rotation: angle})
	world.AddComponent(mid, &domain.Missile{
		TargetID:   targetID,
		OwnerID:    attackerID,
		TeamID:     team,
		Damage:     def.DamagePerShot,
		DamageType: def.DamageType,
		Speed:      speed,
		TurnRate:   3.0,
		Life:       6.0,
		Guidance:   def.Guidance,
	})
}

// subsystemHitDuration is how long a missile strike keeps a subsystem offline (seconds).
const subsystemHitDuration = 3.0

// applySubsystemHit disables one of the target's subsystems from a penetrating missile hit: the
// engines first (thrust cut), then the weapons on a subsequent strike (fire suppressed). Timers
// refresh but don't stack. No-op on entities without a SubsystemState (e.g. stations/loot).
func (s *CombatSystem) applySubsystemHit(world *ecs.World, targetID domain.EntityID) {
	ssVal, ok := world.GetComponent(targetID, domain.SubsystemState{})
	if !ok {
		return
	}
	ss := ssVal.(*domain.SubsystemState)
	if ss.EngineHitTimer <= 0 {
		ss.EngineHitTimer = subsystemHitDuration
	} else {
		ss.WeaponHitTimer = subsystemHitDuration
	}
}

func (s *CombatSystem) raiseFlux(world *ecs.World, id domain.EntityID, amount float32) {
	if amount <= 0 {
		return
	}
	if fVal, ok := world.GetComponent(id, domain.FluxState{}); ok {
		flux := fVal.(*domain.FluxState)
		flux.Current += amount
		if flux.Current >= flux.Capacity {
			triggerOverload(flux)
		}
	}
}

// overloadDuration scales the overload lockout with flux capacity (bigger ships stay down
// longer), clamped to a sane 2–8 s window.
func overloadDuration(capacity float32) float32 {
	d := 2.0 + capacity/500.0
	if d < 2.0 {
		d = 2.0
	}
	if d > 8.0 {
		d = 8.0
	}
	return d
}

// triggerOverload pins flux at capacity and starts a fresh overload lockout. It's a no-op on an
// already-overloaded ship so the timer isn't repeatedly refreshed by further hits.
func triggerOverload(flux *domain.FluxState) {
	flux.Current = flux.Capacity
	if !flux.Overloaded {
		flux.Overloaded = true
		flux.OverloadTimer = overloadDuration(flux.Capacity)
	}
}

// shieldBlocks reports whether the defender's shield covers the incoming direction (from the
// attacker at ax,ay). Omni / unset / full-arc shields cover everything; a directional ("front")
// shield only blocks within Arc degrees of where the ship holds its shield.
func (s *CombatSystem) shieldBlocks(world *ecs.World, defenderID domain.EntityID, sh *domain.Shield, ax, ay float32) bool {
	if sh.Type == "omni" || sh.Type == "" || sh.Arc <= 0 || sh.Arc >= 360 {
		return true
	}
	tVal, ok := world.GetComponent(defenderID, domain.Transform{})
	if !ok {
		return true
	}
	dt := tVal.(*domain.Transform)
	incoming := float32(math.Atan2(float64(ay-dt.Y), float64(ax-dt.X)))
	diff := wrapPi(incoming - shieldFacing(world, defenderID, dt))
	if diff < 0 {
		diff = -diff
	}
	half := sh.Arc * (float32(math.Pi) / 360.0) // Arc (full degrees) → half-arc radians
	return diff <= half
}

// shieldFacing is the direction the ship holds its shield: toward its current combat target if it
// has one (ships angle the shield at whom they fight), else the hull's forward heading. Shared by
// the damage resolver and the snapshot builder so the drawn arc matches what actually blocks.
func shieldFacing(world *ecs.World, defenderID domain.EntityID, dt *domain.Transform) float32 {
	if wVal, ok := world.GetComponent(defenderID, domain.Weapon{}); ok {
		w := wVal.(*domain.Weapon)
		if w.TargetID != 0 {
			if ttVal, ok := world.GetComponent(w.TargetID, domain.Transform{}); ok {
				tt := ttVal.(*domain.Transform)
				return float32(math.Atan2(float64(tt.Y-dt.Y), float64(tt.X-dt.X)))
			}
		}
	}
	return dt.Rotation
}

// wrapPi normalizes an angle to (-π, π].
func wrapPi(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}
