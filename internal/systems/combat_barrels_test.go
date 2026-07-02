package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// A Barrels-N mount fires N shots per trigger (volley), dealing N× damage in one fire() (B5).
func TestBarrels_VolleyMultiShot(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)
	att := domain.EntityID(1)
	tgt := domain.EntityID(2)

	world.RegisterEntityWithID(att, domain.EntityNPC)
	world.AddComponent(att, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(att, &domain.CombatTeam{TeamID: 1})
	def := domain.WeaponDefinition{DamagePerShot: 5, DamageType: domain.DamageKinetic, Range: 1000, Cooldown: 1.0, Barrels: 3}
	world.AddComponent(att, &domain.WeaponGroup{Weapons: []domain.FittedWeaponState{{Definition: def, Ammo: 9999}}})
	world.AddComponent(att, &domain.Weapon{Active: true, TargetID: tgt, Range: 1000})

	world.RegisterEntityWithID(tgt, domain.EntityNPC)
	world.AddComponent(tgt, &domain.Transform{X: 10, Y: 0})
	world.AddComponent(tgt, &domain.Health{Current: 1000, Max: 1000})
	world.AddComponent(tgt, &domain.CombatFx{})

	cs.fire(world, att, 0.05) // one trigger → 3 barrels × 5 = 15
	v, _ := world.GetComponent(tgt, domain.Health{})
	if dmg := 1000 - v.(*domain.Health).Current; dmg != 15 {
		t.Fatalf("expected a 3-shot volley (15 dmg) from one trigger, got %d", dmg)
	}
}
