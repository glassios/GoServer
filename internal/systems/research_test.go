package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func newResearchPlayer(t *testing.T, credits int64) (*ecs.World, domain.EntityID) {
	t.Helper()
	world := ecs.NewWorld()
	pid := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(pid, &domain.PlayerData{Credits: credits})
	world.AddComponent(pid, &domain.PlayerResearch{Completed: map[string]bool{}})
	world.AddComponent(pid, &domain.Cargo{Capacity: 100})
	return world, pid
}

func TestResearch_StartChargesAndCompletes(t *testing.T) {
	world, pid := newResearchPlayer(t, 1000)

	if err := TryStartResearch(world, pid, "adv_weapons"); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	pVal, _ := world.GetComponent(pid, domain.PlayerData{})
	if pVal.(*domain.PlayerData).Credits != 500 { // 1000 - 500 cost
		t.Fatalf("expected 500 credits left, got %d", pVal.(*domain.PlayerData).Credits)
	}

	rs := NewResearchSystem(nil)
	rs.Update(world, 10.0) // not done yet (20s)
	rVal, _ := world.GetComponent(pid, domain.PlayerResearch{})
	if rVal.(*domain.PlayerResearch).HasCompleted("adv_weapons") {
		t.Fatal("should not be complete at 10s")
	}
	rs.Update(world, 11.0) // total 21s -> complete
	if !rVal.(*domain.PlayerResearch).HasCompleted("adv_weapons") {
		t.Fatal("expected adv_weapons completed")
	}
	if rVal.(*domain.PlayerResearch).Active.ProjectID != "" {
		t.Fatal("active research should be cleared on completion")
	}
}

func TestResearch_InsufficientCredits(t *testing.T) {
	world, pid := newResearchPlayer(t, 100)
	if err := TryStartResearch(world, pid, "adv_weapons"); err == nil {
		t.Fatal("expected insufficient-credits rejection")
	}
}

func TestResearch_PrereqGate(t *testing.T) {
	world, pid := newResearchPlayer(t, 10000)
	if err := TryStartResearch(world, pid, "capital_weapons"); err == nil {
		t.Fatal("expected prereq rejection (adv_weapons not done)")
	}
}

func TestResearch_GatesCrafting(t *testing.T) {
	world, pid := newResearchPlayer(t, 0)
	// Stock enough inputs for assemble_heavy_blaster (3 Microchips + 3 EnergyCoils).
	cargoVal, _ := world.GetComponent(pid, domain.Cargo{})
	cargo := cargoVal.(*domain.Cargo)
	cargo.AddResourceTypeQuantity(domain.ResourceMicrochips, 3)
	cargo.AddResourceTypeQuantity(domain.ResourceEnergyCoils, 3)

	// Locked without research.
	if err := TryEnqueueCraft(world, pid, "assemble_heavy_blaster"); err == nil {
		t.Fatal("expected gated recipe to be rejected without research")
	}

	// Unlock and retry.
	rVal, _ := world.GetComponent(pid, domain.PlayerResearch{})
	rVal.(*domain.PlayerResearch).Completed["adv_weapons"] = true
	if err := TryEnqueueCraft(world, pid, "assemble_heavy_blaster"); err != nil {
		t.Fatalf("expected craft to succeed after unlock, got %v", err)
	}
}
