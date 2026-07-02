package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func countMissiles(world *ecs.World) int {
	return len(world.Query(ecs.BuildMask(domain.Missile{})))
}

// A homing missile should fly to its target and detonate, dealing damage (and, being a missile,
// knocking out a subsystem). The target here has no CombatTeam, so point-defense never runs.
func TestMissile_HomesAndHitsTarget(t *testing.T) {
	world := ecs.NewWorld()
	combat := NewCombatSystem(nil)
	ms := NewMissileSystem(combat, 1)

	target := domain.EntityID(1)
	world.RegisterEntityWithID(target, domain.EntityNPC)
	world.AddComponent(target, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(target, &domain.Health{Current: 100, Max: 100})
	world.AddComponent(target, &domain.SubsystemState{})
	world.AddComponent(target, &domain.CombatFx{})

	mid := world.CreateEntity(domain.EntityMissile)
	world.AddComponent(mid, &domain.Transform{X: 100, Y: 0})
	world.AddComponent(mid, &domain.Missile{
		TargetID: target, TeamID: 1, Damage: 40, DamageType: domain.DamageExplosive,
		Speed: 300, TurnRate: 6, Life: 6,
	})

	for i := 0; i < 40 && countMissiles(world) > 0; i++ {
		ms.Update(world, 0.05)
	}

	if countMissiles(world) != 0 {
		t.Fatalf("missile should have detonated, %d still flying", countMissiles(world))
	}
	hVal, _ := world.GetComponent(target, domain.Health{})
	if hVal.(*domain.Health).Current >= 100 {
		t.Errorf("target should have taken missile damage, hp=%d", hVal.(*domain.Health).Current)
	}
	ssVal, _ := world.GetComponent(target, domain.SubsystemState{})
	if ssVal.(*domain.SubsystemState).EngineHitTimer <= 0 {
		t.Errorf("missile hit should have knocked out a subsystem")
	}
}

// Point-defense should shoot down an enemy missile loitering within range of a ship.
func TestMissile_PointDefenseInterceptsEnemyMissile(t *testing.T) {
	world := ecs.NewWorld()
	combat := NewCombatSystem(nil)
	ms := NewMissileSystem(combat, 12345)

	ship := domain.EntityID(2)
	world.RegisterEntityWithID(ship, domain.EntityNPC)
	world.AddComponent(ship, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(ship, &domain.Health{Current: 100, Max: 100})
	world.AddComponent(ship, &domain.CombatTeam{TeamID: 2})

	mid := world.CreateEntity(domain.EntityMissile)
	world.AddComponent(mid, &domain.Transform{X: 30, Y: 0}) // inside pdRange, stationary
	world.AddComponent(mid, &domain.Missile{
		TargetID: 999, TeamID: 1, Damage: 10, DamageType: domain.DamageExplosive,
		Speed: 0, TurnRate: 3, Life: 6,
	})

	for i := 0; i < 120 && countMissiles(world) > 0; i++ {
		ms.Update(world, 0.05)
	}
	if countMissiles(world) != 0 {
		t.Errorf("point-defense should have intercepted the enemy missile")
	}
}

// A missile with no valid target fizzles when its life runs out.
func TestMissile_ExpiresAfterLifetime(t *testing.T) {
	world := ecs.NewWorld()
	combat := NewCombatSystem(nil)
	ms := NewMissileSystem(combat, 1)

	mid := world.CreateEntity(domain.EntityMissile)
	world.AddComponent(mid, &domain.Transform{X: 5000, Y: 5000})
	world.AddComponent(mid, &domain.Missile{TargetID: 999, TeamID: 1, Speed: 0, Life: 0.12})

	ms.Update(world, 0.05)
	ms.Update(world, 0.05)
	ms.Update(world, 0.05)
	if countMissiles(world) != 0 {
		t.Errorf("missile should have expired after its lifetime")
	}
}
