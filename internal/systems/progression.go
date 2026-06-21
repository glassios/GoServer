package systems

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

// BuildPlayerProgressMsg projects a player's skills into a protocol message (level + XP + next-level
// threshold), in the canonical SkillKeys order so the UI is stable.
func BuildPlayerProgressMsg(world *ecs.World, playerID domain.EntityID) *protocol.PlayerProgressMsg {
	msg := &protocol.PlayerProgressMsg{}
	pVal, ok := world.GetComponent(playerID, domain.PlayerProgress{})
	if !ok {
		return msg
	}
	p := pVal.(*domain.PlayerProgress)
	for _, k := range domain.SkillKeys {
		st := p.Skills[k]
		if st == nil {
			st = &domain.SkillState{Level: 1}
		}
		msg.Skills = append(msg.Skills, &protocol.SkillProto{
			Key:    k,
			Level:  st.Level,
			Xp:     st.XP,
			XpNext: domain.XPForNextLevel(st.Level),
		})
	}
	return msg
}

// PublishPlayerProgress pushes S_PLAYER_PROGRESS to the owning player (no-op without a bus).
func PublishPlayerProgress(bus messaging.MessageBus, world *ecs.World, playerID domain.EntityID) {
	if bus == nil {
		return
	}
	msg := BuildPlayerProgressMsg(world, playerID)
	payload, err := proto.Marshal(msg)
	if err != nil {
		return
	}
	packet := &protocol.Packet{Type: protocol.PacketType_S_PLAYER_PROGRESS, Payload: payload}
	data, err := proto.Marshal(packet)
	if err != nil {
		return
	}
	_ = bus.Publish(fmt.Sprintf("player.%d.response", uint64(playerID)), data)
}
