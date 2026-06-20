package systems

import (
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/internal/spatial"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

// RegisterMigrationHandler registers the transfer subscription handler on a System Node.
func RegisterMigrationHandler(bus messaging.MessageBus, world *ecs.World, grid *spatial.HashGrid, systemID uint32, logger *zap.Logger) (messaging.Subscription, error) {
	topic := fmt.Sprintf("system.%d.transfer", systemID)

	sub, err := bus.Reply(topic, func(data []byte) ([]byte, error) {
		var req protocol.SystemTransferRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			logger.Error("Failed to unmarshal transfer request", zap.Error(err))
			return serializeResponse(req.PlayerId, false, "Invalid request format"), nil
		}

		if req.TargetSystemId != systemID {
			logger.Warn("Received transfer request meant for another system",
				zap.Uint32("target", req.TargetSystemId),
				zap.Uint32("current", systemID),
			)
			return serializeResponse(req.PlayerId, false, "Target system ID mismatch"), nil
		}

		logger.Info("Accepting cross-node player transfer",
			zap.Uint64("playerID", req.PlayerId),
			zap.Float32("spawnX", req.SpawnX),
			zap.Float32("spawnY", req.SpawnY),
		)

		// Deserialize and create player entity in this system's world
		playerID := DeserializePlayer(world, req.Payload)

		// Place inside spatial grid
		grid.Insert(playerID, req.SpawnX, req.SpawnY)

		logger.Info("Cross-node player migration completed successfully.", zap.Uint64("playerID", req.PlayerId))

		return serializeResponse(req.PlayerId, true, ""), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to register migration handler reply: %w", err)
	}

	logger.Info("Registered migration handler", zap.String("topic", topic))
	return sub, nil
}

func serializeResponse(playerID uint64, success bool, errMsg string) []byte {
	resp := &protocol.SystemTransferResponse{
		PlayerId:     playerID,
		Success:      success,
		ErrorMessage: errMsg,
	}
	data, _ := proto.Marshal(resp)
	return data
}
