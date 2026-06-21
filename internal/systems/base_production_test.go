package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func TestBaseProduction_CreditsOwner(t *testing.T) {
	world := ecs.NewWorld()
	owner := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(owner, &domain.Cargo{Capacity: 1000})

	base := world.CreateEntity(domain.EntitySpaceBase)
	world.AddComponent(base, &domain.SpaceBase{OwnerID: uint64(owner), Level: 3})

	bp := NewBaseProductionSystem()
	bp.Update(world, 5.0) // one interval

	cargoVal, _ := world.GetComponent(owner, domain.Cargo{})
	got := cargoVal.(*domain.Cargo).GetResourceTypeQuantity("IronPlates")
	if got != 6 { // level 3 * 2
		t.Fatalf("expected 6 IronPlates produced, got %d", got)
	}
}

func TestBaseProduction_NoOwnerNoCrash(t *testing.T) {
	world := ecs.NewWorld()
	base := world.CreateEntity(domain.EntitySpaceBase)
	world.AddComponent(base, &domain.SpaceBase{OwnerID: 99999, Level: 1}) // owner not present

	bp := NewBaseProductionSystem()
	bp.Update(world, 5.0) // must not panic / no-op
}

func TestBaseProduction_RespectsCapacity(t *testing.T) {
	world := ecs.NewWorld()
	owner := world.CreateEntity(domain.EntityPlayer)
	// Capacity 4, already 3 loaded -> only 1 free.
	world.AddComponent(owner, &domain.Cargo{
		Capacity: 4,
		Items:    []domain.ItemInstance{{DefinitionID: 1, Quantity: 3, State: "normal"}},
	})
	base := world.CreateEntity(domain.EntitySpaceBase)
	world.AddComponent(base, &domain.SpaceBase{OwnerID: uint64(owner), Level: 5}) // would make 10

	bp := NewBaseProductionSystem()
	bp.Update(world, 5.0)

	cargoVal, _ := world.GetComponent(owner, domain.Cargo{})
	if got := cargoVal.(*domain.Cargo).GetResourceTypeQuantity("IronPlates"); got != 1 {
		t.Fatalf("expected production clamped to 1 free slot, got %d", got)
	}
}

func TestPlanetProduction_CreditsOwner(t *testing.T) {
	world := ecs.NewWorld()
	owner := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(owner, &domain.PlayerData{Credits: 100})

	planet := world.CreateEntity(domain.EntityPlanet)
	world.AddComponent(planet, &domain.Planet{OwnerID: uint64(owner), DevelopmentLevel: 2})

	bp := NewBaseProductionSystem()
	bp.Update(world, 5.0)

	pVal, _ := world.GetComponent(owner, domain.PlayerData{})
	if got := pVal.(*domain.PlayerData).Credits; got != 100+2*planetCreditsPerLevel {
		t.Fatalf("expected %d credits, got %d", 100+2*planetCreditsPerLevel, got)
	}
}

func TestPlanetProduction_UnclaimedNoIncome(t *testing.T) {
	world := ecs.NewWorld()
	planet := world.CreateEntity(domain.EntityPlanet)
	world.AddComponent(planet, &domain.Planet{OwnerID: 0, DevelopmentLevel: 3}) // unclaimed
	bp := NewBaseProductionSystem()
	bp.Update(world, 5.0) // must not panic / no income
}

func TestBaseProduction_BelowInterval(t *testing.T) {
	world := ecs.NewWorld()
	owner := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(owner, &domain.Cargo{Capacity: 1000})
	base := world.CreateEntity(domain.EntitySpaceBase)
	world.AddComponent(base, &domain.SpaceBase{OwnerID: uint64(owner), Level: 1})

	bp := NewBaseProductionSystem()
	bp.Update(world, 1.0) // below 5s interval -> nothing yet

	cargoVal, _ := world.GetComponent(owner, domain.Cargo{})
	if got := cargoVal.(*domain.Cargo).GetResourceTypeQuantity("IronPlates"); got != 0 {
		t.Fatalf("expected no production before interval, got %d", got)
	}
}
