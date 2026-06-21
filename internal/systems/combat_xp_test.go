package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func TestTallyEnemyKills(t *testing.T) {
	participants := []domain.EntityID{1, 2, 3}
	teams := map[domain.EntityID]uint32{1: 1, 2: 2, 3: 2}
	initial := map[domain.EntityID]int32{1: 3, 2: 2, 3: 2}
	alive := map[domain.EntityID]int32{1: 3, 2: 0, 3: 1}

	killed, survivors := tallyEnemyKills(participants, teams, initial, alive)

	if killed[1] != 3 { // team 1 wiped fleet2 (2) + 1 of fleet3 = 3
		t.Errorf("fleet1 expected 3 enemy kills, got %d", killed[1])
	}
	if killed[2] != 0 || killed[3] != 0 {
		t.Errorf("team2 fleets should have 0 enemy kills (team1 fully alive), got %d/%d", killed[2], killed[3])
	}
	if !survivors[1] || !survivors[2] {
		t.Errorf("both teams should have survivors: %+v", survivors)
	}
}

func TestTallyEnemyKills_CleanWin(t *testing.T) {
	participants := []domain.EntityID{1, 2}
	teams := map[domain.EntityID]uint32{1: 1, 2: 2}
	initial := map[domain.EntityID]int32{1: 2, 2: 2}
	alive := map[domain.EntityID]int32{1: 2, 2: 0}

	killed, survivors := tallyEnemyKills(participants, teams, initial, alive)
	if killed[1] != 2 {
		t.Errorf("expected 2 kills, got %d", killed[1])
	}
	if len(survivors) != 1 || !survivors[1] {
		t.Errorf("expected only team 1 to survive, got %+v", survivors)
	}
}

// Commander combat skill scales the whole fleet's baked weapon damage.
func TestUnpackFleet_CombatDamageBonus(t *testing.T) {
	baseSum := float32(0)
	for _, w := range domain.ComputeStats(domain.DefaultLoadoutForShipType("fighter")).Weapons {
		baseSum += w.Definition.DamagePerShot
	}
	if baseSum == 0 {
		t.Fatal("fighter has no baseline weapon damage")
	}

	src := ecs.NewWorld()
	pid := domain.EntityID(500)
	src.RegisterEntityWithID(pid, domain.EntityPlayer)
	src.AddComponent(pid, &domain.Transform{})
	src.AddComponent(pid, &domain.Fleet{Ships: []domain.FleetShip{
		{ShipID: 1, ShipType: "fighter", Health: 100, MaxHealth: 100},
	}})
	prog := domain.NewPlayerProgress()
	prog.Skills[domain.SkillCombat].Level = 11 // +50% damage
	src.AddComponent(pid, prog)

	dst := ecs.NewWorld()
	ids := UnpackFleet(src, dst, pid, 1, 0, 0, 0)
	if len(ids) != 1 {
		t.Fatalf("expected 1 ship unpacked, got %d", len(ids))
	}

	wgVal, ok := dst.GetComponent(ids[0], domain.WeaponGroup{})
	if !ok {
		t.Fatal("flagship missing WeaponGroup")
	}
	bakedSum := float32(0)
	for _, w := range wgVal.(*domain.WeaponGroup).Weapons {
		bakedSum += w.Definition.DamagePerShot
	}
	want := baseSum * 1.5
	if bakedSum < want-0.5 || bakedSum > want+0.5 {
		t.Fatalf("expected baked damage ~%.1f (1.5x of %.1f), got %.1f", want, baseSum, bakedSum)
	}
}
