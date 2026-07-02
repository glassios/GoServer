package systems

import (
	"math"
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// A fixed hardpoint (narrow arc) should only fire when the ship is pointed at the target (B5).
func TestMountArc_HardpointFiresOnlyForward(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	att := domain.EntityID(1)
	tgt := domain.EntityID(2)

	world.RegisterEntityWithID(att, domain.EntityNPC)
	world.AddComponent(att, &domain.Transform{X: 0, Y: 0, Rotation: 0}) // facing +X
	world.AddComponent(att, &domain.CombatTeam{TeamID: 1})
	def := domain.WeaponDefinition{DamagePerShot: 5, DamageType: domain.DamageKinetic, Range: 1000, Cooldown: 0}
	world.AddComponent(att, &domain.WeaponGroup{Weapons: []domain.FittedWeaponState{
		{Definition: def, Ammo: 9999, ArcHalf: domain.MountArcHalf("HARDPOINT")},
	}})
	world.AddComponent(att, &domain.Weapon{Active: true, TargetID: tgt, Range: 1000})

	world.RegisterEntityWithID(tgt, domain.EntityNPC)
	world.AddComponent(tgt, &domain.Transform{X: 100, Y: 0}) // directly ahead (+X)
	world.AddComponent(tgt, &domain.Health{Current: 1000, Max: 1000})
	world.AddComponent(tgt, &domain.CombatFx{})

	hp := func() int32 {
		v, _ := world.GetComponent(tgt, domain.Health{})
		return v.(*domain.Health).Current
	}

	cs.fire(world, att, 0.05) // target in the forward cone → fires
	if hp() == 1000 {
		t.Fatalf("hardpoint should hit a target dead ahead")
	}
	before := hp()

	// Rotate the ship to face away (+X target is now behind the forward cone).
	aTVal, _ := world.GetComponent(att, domain.Transform{})
	aTVal.(*domain.Transform).Rotation = math.Pi
	cs.fire(world, att, 0.05)
	if hp() != before {
		t.Errorf("hardpoint should NOT fire at a target outside its arc (hp %d -> %d)", before, hp())
	}
}
