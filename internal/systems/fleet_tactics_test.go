package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// Per-ship role/strategy must survive a jump-gate migration (serialize -> deserialize).
func TestJumpGate_MigrationPreservesTactics(t *testing.T) {
	src := ecs.NewWorld()
	pid := domain.EntityID(888)
	src.RegisterEntityWithID(pid, domain.EntityPlayer)
	src.AddComponent(pid, &domain.Transform{X: 1, Y: 2})
	src.AddComponent(pid, &domain.Health{Current: 100, Max: 100})
	src.AddComponent(pid, &domain.Shield{Current: 50, Max: 50})
	src.AddComponent(pid, &domain.PlayerData{Name: "Cmdr", Credits: 10})
	src.AddComponent(pid, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100, Role: domain.RoleTank, Strategy: domain.StanceDefense},
		{ShipID: 2, ShipType: "miner", Health: 80, MaxHealth: 80, Role: domain.RoleRepair, Strategy: domain.StanceRetreat},
	}})

	payload := SerializePlayer(src, pid)

	dst := ecs.NewWorld()
	newID := DeserializePlayer(dst, payload)

	flVal, ok := dst.GetComponent(newID, domain.Fleet{})
	if !ok {
		t.Fatal("expected Fleet after migration")
	}
	fleet := flVal.(*domain.Fleet)
	if len(fleet.Ships) != 2 {
		t.Fatalf("expected 2 ships, got %d", len(fleet.Ships))
	}
	if fleet.Ships[0].Role != domain.RoleTank || fleet.Ships[0].Strategy != domain.StanceDefense {
		t.Errorf("flagship tactics not preserved: %+v", fleet.Ships[0])
	}
	if fleet.Ships[1].Role != domain.RoleRepair || fleet.Ships[1].Strategy != domain.StanceRetreat {
		t.Errorf("escort tactics not preserved: %+v", fleet.Ships[1])
	}
}

// ResolveTactics applies explicit values and fills defaults for blanks.
func TestResolveTactics_Defaults(t *testing.T) {
	r, s := domain.ResolveTactics("", "", 1)
	if r == "" || s != domain.StanceAttack {
		t.Errorf("expected defaulted role and attack stance, got role=%q stance=%q", r, s)
	}
	r2, s2 := domain.ResolveTactics(domain.RoleSupport, domain.StanceRetreat, 0)
	if r2 != domain.RoleSupport || s2 != domain.StanceRetreat {
		t.Errorf("explicit tactics changed: role=%q stance=%q", r2, s2)
	}
}
