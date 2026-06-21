package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/pkg/mathutil"
)

func addEnemy(world *ecs.World, id domain.EntityID, team uint32, x, y float32, hp int32, role string) {
	world.RegisterEntityWithID(id, domain.EntityNPC)
	world.AddComponent(id, &domain.Transform{X: x, Y: y})
	world.AddComponent(id, &domain.Health{Current: hp, Max: 100})
	world.AddComponent(id, &domain.CombatTeam{TeamID: team})
	if role != "" {
		world.AddComponent(id, &domain.CombatRole{Role: role})
	}
}

// Focus fire: with all else equal, the weakest enemy is chosen.
func TestSelectCombatTarget_PrefersWeakest(t *testing.T) {
	world := ecs.NewWorld()
	self := domain.EntityID(1)
	world.RegisterEntityWithID(self, domain.EntityNPC)
	world.AddComponent(self, &domain.CombatTeam{TeamID: 1})

	addEnemy(world, 2, 2, 100, 0, 90, "") // healthy, near
	addEnemy(world, 3, 2, 110, 0, 20, "") // weak, slightly farther

	id, _, _, found := selectCombatTarget(world, self, 1, mathutil.NewVec2(0, 0))
	if !found || id != 3 {
		t.Errorf("expected to focus the weakest enemy (3), got %d (found=%v)", id, found)
	}
}

// Taunt: a tank is preferred over a similarly-placed non-tank.
func TestSelectCombatTarget_TankDrawsFire(t *testing.T) {
	world := ecs.NewWorld()
	self := domain.EntityID(1)
	world.RegisterEntityWithID(self, domain.EntityNPC)
	world.AddComponent(self, &domain.CombatTeam{TeamID: 1})

	addEnemy(world, 2, 2, 100, 0, 80, "")               // dps, same distance/hp
	addEnemy(world, 3, 2, 100, 5, 80, domain.RoleTank)  // tank, ~same spot

	id, _, _, found := selectCombatTarget(world, self, 1, mathutil.NewVec2(0, 0))
	if !found || id != 3 {
		t.Errorf("expected tank (3) to draw fire, got %d", id)
	}
}

// Repair role restores a wounded ally's hull over time.
func TestRoleAbility_RepairHealsAlly(t *testing.T) {
	world := ecs.NewWorld()
	cs := NewCombatSystem(nil)

	healer := domain.EntityID(1)
	world.RegisterEntityWithID(healer, domain.EntityNPC)
	world.AddComponent(healer, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(healer, &domain.CombatTeam{TeamID: 1})
	world.AddComponent(healer, &domain.CombatRole{Role: domain.RoleRepair})

	ally := domain.EntityID(2)
	world.RegisterEntityWithID(ally, domain.EntityNPC)
	world.AddComponent(ally, &domain.Transform{X: 50, Y: 0})
	world.AddComponent(ally, &domain.CombatTeam{TeamID: 1})
	world.AddComponent(ally, &domain.Health{Current: 40, Max: 100})

	// Several ticks of repair should raise the ally's hull and set the assist target.
	for i := 0; i < 30; i++ {
		cs.roleAbility(world, healer, 0.05)
	}

	h, _ := world.GetComponent(ally, domain.Health{})
	if h.(*domain.Health).Current <= 40 {
		t.Errorf("expected ally hull to be repaired above 40, got %d", h.(*domain.Health).Current)
	}
	r, _ := world.GetComponent(healer, domain.CombatRole{})
	if r.(*domain.CombatRole).AssistTargetID != ally {
		t.Errorf("expected healer AssistTargetID=%d, got %d", ally, r.(*domain.CombatRole).AssistTargetID)
	}
}
