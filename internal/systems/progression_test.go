package systems

import (
	"testing"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
)

func TestPlayerProgress_AddXPLevels(t *testing.T) {
	p := domain.NewPlayerProgress()
	// Level 1 needs 100 to reach 2; give 250 -> level 2 (uses 100), then level 3 needs 200 (50 left short).
	gained := p.AddXP(domain.SkillMining, 250)
	if gained != 1 {
		t.Fatalf("expected 1 level gained, got %d", gained)
	}
	if p.Level(domain.SkillMining) != 2 {
		t.Fatalf("expected level 2, got %d", p.Level(domain.SkillMining))
	}
	if p.Skills[domain.SkillMining].XP != 150 {
		t.Fatalf("expected 150 leftover XP, got %d", p.Skills[domain.SkillMining].XP)
	}
}

func TestPlayerProgress_Bonuses(t *testing.T) {
	p := domain.NewPlayerProgress()
	p.Skills[domain.SkillMining].Level = 6 // +50% yield
	if got := p.MiningYieldMult(); got < 1.49 || got > 1.51 {
		t.Errorf("expected ~1.5 mining mult, got %f", got)
	}
	p.Skills[domain.SkillEngineering].Level = 5 // -20% craft time
	if got := p.CraftTimeMult(); got < 0.79 || got > 0.81 {
		t.Errorf("expected ~0.8 craft mult, got %f", got)
	}
}

func TestMining_YieldAndXP(t *testing.T) {
	world := ecs.NewWorld()
	miner := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(miner, &domain.Transform{X: 0, Y: 0})
	world.AddComponent(miner, &domain.Cargo{Capacity: 1000})
	prog := domain.NewPlayerProgress()
	prog.Skills[domain.SkillMining].Level = 6 // 1.5x yield
	world.AddComponent(miner, prog)

	ast := world.CreateEntity(domain.EntityAsteroid)
	world.AddComponent(ast, &domain.Transform{X: 5, Y: 5})
	world.AddComponent(ast, &domain.AsteroidResource{Type: domain.ResourceIron, Amount: 100})

	world.AddComponent(miner, &domain.MiningLaser{Power: 10, Range: 50, Active: true, TargetID: ast})

	ms := NewMiningSystem(nil)
	ms.Update(world, 1.0)

	cargoVal, _ := world.GetComponent(miner, domain.Cargo{})
	if got := cargoVal.(*domain.Cargo).GetResourceTypeQuantity(domain.ResourceIron); got != 15 {
		t.Fatalf("expected 15 iron (10 base * 1.5), got %d", got)
	}
	if prog.Skills[domain.SkillMining].XP != 15 {
		t.Fatalf("expected 15 mining XP, got %d", prog.Skills[domain.SkillMining].XP)
	}
}

func TestProduction_EngineeringXPAndSpeed(t *testing.T) {
	world := ecs.NewWorld()
	pid := world.CreateEntity(domain.EntityPlayer)
	world.AddComponent(pid, &domain.Cargo{
		Items:    []domain.ItemInstance{{DefinitionID: 1, Quantity: 4, State: "normal"}},
		Capacity: 100,
	})
	prog := domain.NewPlayerProgress()
	prog.Skills[domain.SkillEngineering].Level = 5 // 0.8x craft time
	world.AddComponent(pid, prog)

	if err := TryEnqueueCraft(world, pid, "refine_iron"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	qVal, _ := world.GetComponent(pid, domain.CraftQueue{})
	job := qVal.(*domain.CraftQueue).Jobs[0]
	if job.TotalTime < 2.39 || job.TotalTime > 2.41 { // 3s * 0.8
		t.Fatalf("expected ~2.4s craft time, got %f", job.TotalTime)
	}

	prod := NewProductionSystem(nil)
	prod.Update(world, float64(job.TotalTime)+0.01)
	if prog.Skills[domain.SkillEngineering].XP != 10 { // tier 1 * 10
		t.Fatalf("expected 10 engineering XP, got %d", prog.Skills[domain.SkillEngineering].XP)
	}
}
