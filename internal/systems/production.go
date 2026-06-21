package systems

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

// ProductionSystem (Phase 3) is the data-driven recipe runner for player crafting. It advances
// each player's CraftQueue head job; inputs were consumed at enqueue time, so on completion it
// just delivers the recipe outputs to the player's cargo. It pushes S_PRODUCTION_STATUS to the
// owning player on completion and on a ~1s heartbeat while any job is active, so the client's
// production panel shows live progress.
type ProductionSystem struct {
	bus       messaging.MessageBus
	heartbeat float64
}

func NewProductionSystem(bus messaging.MessageBus) *ProductionSystem {
	return &ProductionSystem{bus: bus}
}

func (s *ProductionSystem) Name() string  { return "ProductionSystem" }
func (s *ProductionSystem) Priority() int { return 82 }

func (s *ProductionSystem) Update(world *ecs.World, dt float64) {
	mask := ecs.BuildMask(domain.CraftQueue{}, domain.Cargo{})
	ents := world.Query(mask)
	if len(ents) == 0 {
		return
	}

	s.heartbeat += dt
	emitHeartbeat := s.heartbeat >= 1.0
	if emitHeartbeat {
		s.heartbeat = 0
	}

	for _, id := range ents {
		qVal, _ := world.GetComponent(id, domain.CraftQueue{})
		q := qVal.(*domain.CraftQueue)
		if len(q.Jobs) == 0 {
			continue
		}
		cargoVal, _ := world.GetComponent(id, domain.Cargo{})
		cargo := cargoVal.(*domain.Cargo)

		job := &q.Jobs[0]
		job.Progress += float32(dt)

		changed := false
		if job.Progress >= job.TotalTime {
			if r := domain.RecipeByID(job.RecipeID); r != nil {
				for res, qty := range r.Outputs {
					cargo.AddResourceTypeQuantity(domain.ResourceType(res), qty)
				}
				// Award engineering XP (scaled by recipe tier) and push the new progress.
				if pVal, hasP := world.GetComponent(id, domain.PlayerProgress{}); hasP {
					pVal.(*domain.PlayerProgress).AddXP(domain.SkillEngineering, r.Tier*10)
					PublishPlayerProgress(s.bus, world, id)
				}
			}
			q.Jobs = q.Jobs[1:]
			changed = true
		}

		if (changed || emitHeartbeat) && s.bus != nil {
			s.publishStatus(world, id)
		}
	}
}

func (s *ProductionSystem) publishStatus(world *ecs.World, playerID domain.EntityID) {
	status := BuildProductionStatus(world, playerID)
	payload, err := proto.Marshal(status)
	if err != nil {
		return
	}
	packet := &protocol.Packet{Type: protocol.PacketType_S_PRODUCTION_STATUS, Payload: payload}
	packetData, err := proto.Marshal(packet)
	if err != nil {
		return
	}
	_ = s.bus.Publish(fmt.Sprintf("player.%d.response", uint64(playerID)), packetData)
}

// BuildProductionStatus projects a player's craft queue plus the full recipe catalog into a
// protocol message. Shared by the worldnode handler/auth path and ProductionSystem's heartbeat.
func BuildProductionStatus(world *ecs.World, playerID domain.EntityID) *protocol.ProductionStatus {
	status := &protocol.ProductionStatus{}

	if qVal, ok := world.GetComponent(playerID, domain.CraftQueue{}); ok {
		q := qVal.(*domain.CraftQueue)
		for _, j := range q.Jobs {
			status.Queue = append(status.Queue, &protocol.CraftJobProto{
				RecipeId:  j.RecipeID,
				Name:      j.Name,
				Progress:  j.Progress,
				TotalTime: j.TotalTime,
			})
		}
	}

	for i := range domain.StockRecipes {
		r := &domain.StockRecipes[i]
		status.Recipes = append(status.Recipes, &protocol.RecipeProto{
			Id:          r.ID,
			Name:        r.Name,
			Tier:        r.Tier,
			Inputs:      r.Inputs,
			Outputs:     r.Outputs,
			TimeSeconds: r.TimeSeconds,
		})
	}
	return status
}

// TryEnqueueCraft validates a recipe against the player's cargo, consumes the inputs, and appends
// a job to the player's craft queue (creating it if absent). Returns an error (suitable for a
// system-chat message) when the recipe is unknown or inputs are insufficient.
func TryEnqueueCraft(world *ecs.World, playerID domain.EntityID, recipeID string) error {
	recipe := domain.RecipeByID(recipeID)
	if recipe == nil {
		return fmt.Errorf("неизвестный рецепт")
	}
	cargoVal, ok := world.GetComponent(playerID, domain.Cargo{})
	if !ok {
		return fmt.Errorf("нет грузового отсека")
	}
	cargo := cargoVal.(*domain.Cargo)

	for res, qty := range recipe.Inputs {
		if cargo.GetResourceTypeQuantity(domain.ResourceType(res)) < qty {
			return fmt.Errorf("недостаточно ресурсов: %s", res)
		}
	}
	for res, qty := range recipe.Inputs {
		cargo.RemoveResourceTypeQuantity(domain.ResourceType(res), qty)
	}

	qVal, has := world.GetComponent(playerID, domain.CraftQueue{})
	var q *domain.CraftQueue
	if has {
		q = qVal.(*domain.CraftQueue)
	} else {
		q = &domain.CraftQueue{}
		world.AddComponent(playerID, q)
	}

	// Engineering skill speeds up crafting (Phase 3).
	totalTime := recipe.TimeSeconds
	if pVal, hasP := world.GetComponent(playerID, domain.PlayerProgress{}); hasP {
		totalTime *= pVal.(*domain.PlayerProgress).CraftTimeMult()
	}

	q.Jobs = append(q.Jobs, domain.CraftJob{
		RecipeID:  recipe.ID,
		Name:      recipe.Name,
		Progress:  0,
		TotalTime: totalTime,
	})
	return nil
}
