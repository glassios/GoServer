package systems

import (
	"math"
	"math/rand"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// MissileSystem simulates live, flying missile entities inside a combat instance (Phase B4).
// Missile-class mounts spawn a domain.Missile entity (see CombatSystem.spawnMissile) instead of
// dealing instant damage; this system homes each missile toward its target, applies the damage on
// proximity (reusing CombatSystem.applyDamage, so directional shields + subsystem hits still work),
// expires missiles that run out of life, and lets ships shoot down incoming enemy missiles with
// point-defense. Missiles carry Transform (no Velocity) so MovementSystem ignores them — this
// system integrates their position directly.
type MissileSystem struct {
	combat *CombatSystem
	rng    *rand.Rand
}

const (
	missileHitRadius   = 46.0 // world units: proximity at which a missile detonates on its target
	pdRange            = 260.0 // world units: point-defense engagement radius
	pdInterceptPerSec  = 1.4  // expected intercepts/sec a ship rolls against each in-range enemy missile
)

func NewMissileSystem(combat *CombatSystem, seed int64) *MissileSystem {
	return &MissileSystem{combat: combat, rng: rand.New(rand.NewSource(seed))}
}

func (s *MissileSystem) Name() string { return "MissileSystem" }

func (s *MissileSystem) Priority() int { return 75 } // after CombatSystem (80), before cleanup

func (s *MissileSystem) Update(world *ecs.World, dt float64) {
	dtf := float32(dt)

	for _, id := range world.Query(ecs.BuildMask(domain.Missile{}, domain.Transform{})) {
		mVal, _ := world.GetComponent(id, domain.Missile{})
		m := mVal.(*domain.Missile)
		tVal, _ := world.GetComponent(id, domain.Transform{})
		t := tVal.(*domain.Transform)

		m.Life -= dtf
		m.Age += dtf
		if m.Life <= 0 {
			world.DestroyEntity(id)
			continue
		}

		// Home toward a living target; if the target is gone/dead, coast on the current heading.
		heading := t.Rotation
		targetAlive := false
		if ttVal, ok := world.GetComponent(m.TargetID, domain.Transform{}); ok {
			tt := ttVal.(*domain.Transform)
			if hVal, ok2 := world.GetComponent(m.TargetID, domain.Health{}); !ok2 || hVal.(*domain.Health).Current > 0 {
				targetAlive = true
				desired := float32(math.Atan2(float64(tt.Y-t.Y), float64(tt.X-t.X)))
				// Weaving guidance (B4): bias the aim by a decaying sine so the missile snakes toward
				// the target (harder to shoot down); the weave shrinks as it closes in.
				if m.Guidance == "weave" {
					dx2, dy2 := tt.X-t.X, tt.Y-t.Y
					closeAmp := float32(math.Min(1, math.Hypot(float64(dx2), float64(dy2))/400.0))
					desired += float32(math.Sin(float64(m.Age)*9.0)) * 0.6 * closeAmp
				}
				heading = steerAngle(t.Rotation, desired, m.TurnRate*dtf)
			}
			// Detonate on proximity to the (still-present) target.
			dx, dy := tt.X-t.X, tt.Y-t.Y
			if dx*dx+dy*dy <= missileHitRadius*missileHitRadius {
				s.combat.applyDamage(world, m.TargetID, t.X, t.Y, m.Damage, m.DamageType, domain.WeaponClassMissile)
				world.DestroyEntity(id)
				continue
			}
		}
		_ = targetAlive

		t.Rotation = heading
		t.X += float32(math.Cos(float64(heading))) * m.Speed * dtf
		t.Y += float32(math.Sin(float64(heading))) * m.Speed * dtf
	}

	s.pointDefense(world, dtf)
}

// pointDefense lets each living combat ship roll to shoot down enemy missiles within pdRange.
func (s *MissileSystem) pointDefense(world *ecs.World, dtf float32) {
	missiles := world.Query(ecs.BuildMask(domain.Missile{}, domain.Transform{}))
	if len(missiles) == 0 {
		return
	}
	destroyed := make(map[domain.EntityID]bool, len(missiles))

	for _, shipID := range world.Query(ecs.BuildMask(domain.Transform{}, domain.CombatTeam{}, domain.Health{})) {
		hVal, _ := world.GetComponent(shipID, domain.Health{})
		if hVal.(*domain.Health).Current <= 0 {
			continue
		}
		teamVal, _ := world.GetComponent(shipID, domain.CombatTeam{})
		team := teamVal.(*domain.CombatTeam).TeamID
		stVal, _ := world.GetComponent(shipID, domain.Transform{})
		st := stVal.(*domain.Transform)

		for _, mid := range missiles {
			if destroyed[mid] {
				continue
			}
			mVal, ok := world.GetComponent(mid, domain.Missile{})
			if !ok {
				continue
			}
			m := mVal.(*domain.Missile)
			if m.TeamID == team {
				continue // don't shoot down our own team's missiles
			}
			mtVal, _ := world.GetComponent(mid, domain.Transform{})
			mt := mtVal.(*domain.Transform)
			dx, dy := mt.X-st.X, mt.Y-st.Y
			if dx*dx+dy*dy > pdRange*pdRange {
				continue
			}
			if s.rng.Float32() < pdInterceptPerSec*dtf {
				destroyed[mid] = true
			}
		}
	}

	for mid := range destroyed {
		world.DestroyEntity(mid)
	}
}

// steerAngle rotates cur toward desired by at most maxDelta radians (shortest way).
func steerAngle(cur, desired, maxDelta float32) float32 {
	d := wrapPi(desired - cur)
	if d > maxDelta {
		d = maxDelta
	} else if d < -maxDelta {
		d = -maxDelta
	}
	return cur + d
}
