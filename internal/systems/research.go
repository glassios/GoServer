package systems

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

// ResearchSystem (Phase 3) advances each player's active research project. On completion the
// project is marked done (unlocking its recipes) and the client is refreshed with both the new
// research status and the production status (so freshly-unlocked recipes appear).
type ResearchSystem struct {
	bus       messaging.MessageBus
	heartbeat float64
}

func NewResearchSystem(bus messaging.MessageBus) *ResearchSystem {
	return &ResearchSystem{bus: bus}
}

func (s *ResearchSystem) Name() string  { return "ResearchSystem" }
func (s *ResearchSystem) Priority() int { return 83 }

func (s *ResearchSystem) Update(world *ecs.World, dt float64) {
	ents := world.Query(ecs.BuildMask(domain.PlayerResearch{}))
	if len(ents) == 0 {
		return
	}

	s.heartbeat += dt
	emitHeartbeat := s.heartbeat >= 1.0
	if emitHeartbeat {
		s.heartbeat = 0
	}

	for _, id := range ents {
		rVal, _ := world.GetComponent(id, domain.PlayerResearch{})
		r := rVal.(*domain.PlayerResearch)
		if r.Active.ProjectID == "" {
			continue
		}

		r.Active.Progress += float32(dt)
		done := false
		if r.Active.Progress >= r.Active.TotalTime {
			if r.Completed == nil {
				r.Completed = map[string]bool{}
			}
			r.Completed[r.Active.ProjectID] = true
			r.Active = domain.ActiveResearch{}
			done = true
		}

		if (done || emitHeartbeat) && s.bus != nil {
			PublishResearchStatus(s.bus, world, id)
			if done {
				// Recipe lock state changed — refresh the production panel too.
				publishProductionStatusFor(s.bus, world, id)
			}
		}
	}
}

// BuildResearchStatus projects the research catalog into a per-player status message.
func BuildResearchStatus(world *ecs.World, playerID domain.EntityID) *protocol.ResearchStatus {
	status := &protocol.ResearchStatus{}
	var research *domain.PlayerResearch
	if rVal, ok := world.GetComponent(playerID, domain.PlayerResearch{}); ok {
		research = rVal.(*domain.PlayerResearch)
	} else {
		research = domain.NewPlayerResearch()
	}

	for i := range domain.StockResearch {
		p := &domain.StockResearch[i]
		var st string
		var progress, total float32
		switch {
		case research.HasCompleted(p.ID):
			st = "completed"
		case research.Active.ProjectID == p.ID:
			st = "active"
			progress = research.Active.Progress
			total = research.Active.TotalTime
		case prereqsMet(research, p):
			st = "available"
		default:
			st = "locked"
		}
		status.Projects = append(status.Projects, &protocol.ResearchProjectProto{
			Id:          p.ID,
			Name:        p.Name,
			Cost:        p.Cost,
			TimeSeconds: p.TimeSeconds,
			Prereqs:     append([]string{}, p.Prereqs...),
			Unlocks:     append([]string{}, p.Unlocks...),
			Status:      st,
			Progress:    progress,
			TotalTime:   total,
		})
	}
	return status
}

func prereqsMet(r *domain.PlayerResearch, p *domain.ResearchProject) bool {
	for _, req := range p.Prereqs {
		if !r.HasCompleted(req) {
			return false
		}
	}
	return true
}

// PublishResearchStatus pushes S_RESEARCH_STATUS to the owning player.
func PublishResearchStatus(bus messaging.MessageBus, world *ecs.World, playerID domain.EntityID) {
	if bus == nil {
		return
	}
	msg := BuildResearchStatus(world, playerID)
	payload, err := proto.Marshal(msg)
	if err != nil {
		return
	}
	packet := &protocol.Packet{Type: protocol.PacketType_S_RESEARCH_STATUS, Payload: payload}
	data, err := proto.Marshal(packet)
	if err != nil {
		return
	}
	_ = bus.Publish(fmt.Sprintf("player.%d.response", uint64(playerID)), data)
}

// publishProductionStatusFor pushes S_PRODUCTION_STATUS (recipe lock state may have changed).
func publishProductionStatusFor(bus messaging.MessageBus, world *ecs.World, playerID domain.EntityID) {
	if bus == nil {
		return
	}
	status := BuildProductionStatus(world, playerID)
	payload, err := proto.Marshal(status)
	if err != nil {
		return
	}
	packet := &protocol.Packet{Type: protocol.PacketType_S_PRODUCTION_STATUS, Payload: payload}
	data, err := proto.Marshal(packet)
	if err != nil {
		return
	}
	_ = bus.Publish(fmt.Sprintf("player.%d.response", uint64(playerID)), data)
}

// TryStartResearch validates and starts a research project for a player (charging credits).
// Returns an error suitable for a system-chat message.
func TryStartResearch(world *ecs.World, playerID domain.EntityID, projectID string) error {
	project := domain.ResearchByID(projectID)
	if project == nil {
		return fmt.Errorf("неизвестный проект")
	}

	rVal, has := world.GetComponent(playerID, domain.PlayerResearch{})
	var research *domain.PlayerResearch
	if has {
		research = rVal.(*domain.PlayerResearch)
	} else {
		research = domain.NewPlayerResearch()
		world.AddComponent(playerID, research)
	}

	if research.HasCompleted(projectID) {
		return fmt.Errorf("уже изучено")
	}
	if research.Active.ProjectID != "" {
		return fmt.Errorf("уже идёт другое исследование")
	}
	if !prereqsMet(research, project) {
		return fmt.Errorf("не выполнены требования")
	}

	pVal, ok := world.GetComponent(playerID, domain.PlayerData{})
	if !ok {
		return fmt.Errorf("нет данных игрока")
	}
	player := pVal.(*domain.PlayerData)
	if player.Credits < project.Cost {
		return fmt.Errorf("недостаточно кредитов (%d)", project.Cost)
	}
	player.Credits -= project.Cost

	research.Active = domain.ActiveResearch{
		ProjectID: project.ID,
		Progress:  0,
		TotalTime: project.TimeSeconds,
	}
	return nil
}
