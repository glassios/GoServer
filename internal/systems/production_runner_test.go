package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

// TryEnqueueCraft consumes inputs and queues a job; ProductionSystem delivers outputs on completion.
func TestProduction_CraftConsumeAndProduce(t *testing.T) {
	world := ecs.NewWorld()
	pid := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(pid, &domain.Cargo{
		Items:    []domain.ItemInstance{{DefinitionID: 1, Quantity: 4, State: "normal"}}, // 4 Iron (def 1)
		Capacity: 100,
	})

	// refine_iron: 2 Iron -> 1 IronPlates, 3s
	if err := TryEnqueueCraft(world, pid, "refine_iron"); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	cargoVal, _ := world.GetComponent(pid, domain.Cargo{})
	cargo := cargoVal.(*domain.Cargo)
	if got := cargo.GetResourceTypeQuantity(domain.ResourceIron); got != 2 {
		t.Fatalf("expected 2 Iron after consuming inputs, got %d", got)
	}

	prod := NewProductionSystem(nil)
	prod.Update(world, 1.0)
	if got := cargo.GetResourceTypeQuantity("IronPlates"); got != 0 {
		t.Fatalf("job should not be done at 1s, got %d IronPlates", got)
	}
	prod.Update(world, 2.0) // total 3s -> complete
	if got := cargo.GetResourceTypeQuantity("IronPlates"); got != 1 {
		t.Fatalf("expected 1 IronPlates delivered, got %d", got)
	}

	qVal, _ := world.GetComponent(pid, domain.CraftQueue{})
	if len(qVal.(*domain.CraftQueue).Jobs) != 0 {
		t.Errorf("expected queue empty after completion")
	}
}

func TestProduction_InsufficientInputs(t *testing.T) {
	world := ecs.NewWorld()
	pid := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(pid, &domain.Cargo{
		Items:    []domain.ItemInstance{{DefinitionID: 1, Quantity: 1, State: "normal"}}, // only 1 Iron
		Capacity: 100,
	})
	if err := TryEnqueueCraft(world, pid, "refine_iron"); err == nil {
		t.Fatal("expected insufficient-inputs rejection")
	}
}

func TestProduction_UnknownRecipe(t *testing.T) {
	world := ecs.NewWorld()
	pid := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(pid, &domain.Cargo{Capacity: 100})
	if err := TryEnqueueCraft(world, pid, "does_not_exist"); err == nil {
		t.Fatal("expected unknown-recipe rejection")
	}
}
