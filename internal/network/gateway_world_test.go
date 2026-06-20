package network

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/domain"
	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/gameloop"
	"github.com/Home/galaxy-mmo/internal/messaging"
	"github.com/Home/galaxy-mmo/internal/persistence"
	"github.com/Home/galaxy-mmo/internal/spatial"
	"github.com/Home/galaxy-mmo/internal/systems"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

func TestGatewayWorldNode_Integration(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 1. Shared Messaging Bus
	bus := messaging.NewMockMessageBus()
	defer bus.Close()

	// 2. World Node Setup (System 1)
	systemID := uint32(1)
	world := ecs.NewWorld()
	grid := spatial.NewHashGrid(1000)

	moveSys := systems.NewMovementSystem(50000, 50000)
	ecsSystems := []ecs.System{moveSys}
	loop := gameloop.NewGameLoop(world, ecsSystems, 20, logger)

	// Subscribe to inputs
	inputTopic := fmt.Sprintf("system.%d.input", systemID)
	_, err := bus.Subscribe(inputTopic, func(msg *messaging.Message) {
		var cmd protocol.ServerCommand
		if err := proto.Unmarshal(msg.Data, &cmd); err != nil {
			return
		}

		playerID := domain.EntityID(cmd.PlayerId)
		switch cmd.Type {
		case protocol.PacketType_C_AUTH_REQUEST:
			if _, exists := world.GetEntityType(playerID); !exists {
				world.RegisterEntityWithID(playerID, domain.EntityPlayer)
				world.AddComponent(playerID, &domain.Transform{X: 100, Y: 100})
				world.AddComponent(playerID, &domain.Velocity{X: 0, Y: 0})
				world.AddComponent(playerID, &domain.Health{Current: 100, Max: 100})
				world.AddComponent(playerID, &domain.Shield{Current: 50, Max: 50, RegenRate: 1.0})
				world.AddComponent(playerID, &domain.ShipConfig{ShipType: "fighter", MaxSpeed: 100, TurnRate: 1.5})
				world.AddComponent(playerID, &domain.Visibility{Radius: 1500.0, VisibleEntities: make(map[domain.EntityID]struct{})})
				world.AddComponent(playerID, &domain.PlayerData{AccountID: uint64(playerID), Name: string(cmd.Payload), Credits: 1000})
				grid.Insert(playerID, 100, 100)
			}
		case protocol.PacketType_C_MOVE_INPUT:
			var moveInput protocol.MoveInput
			if err := proto.Unmarshal(cmd.Payload, &moveInput); err == nil {
				loop.EnqueueCommand(gameloop.Command{
					PlayerID: playerID,
					Type:     "move",
					Payload: domain.Velocity{
						X: moveInput.X,
						Y: moveInput.Y,
					},
				})
			}
		}
	})
	if err != nil {
		t.Fatalf("Failed to subscribe on world node: %v", err)
	}

	// Publish outputs (snapshots)
	outputTopic := fmt.Sprintf("system.%d.output", systemID)
	loop.OnSnapshot = func(tick uint64) {
		var entSnaps []*protocol.EntitySnapshot
		allEntities := world.Query(0)
		for _, id := range allEntities {
			snap := BuildEntitySnapshot(world, id)
			if snap != nil {
				entSnaps = append(entSnaps, snap)
			}
		}

		worldSnap := &protocol.WorldSnapshot{
			Tick:     tick,
			Entities: entSnaps,
		}
		data, _ := proto.Marshal(worldSnap)
		_ = bus.Publish(outputTopic, data)
	}

	// Run Game Loop in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// 3. Gateway Server Setup
	cache := &MockSessionCache{}
	gateway := NewServer("127.0.0.1:0", nil, cache, logger)

	routingTable := make(map[domain.EntityID]uint32)
	var rtMu sync.RWMutex

	gateway.OnPlayerAuth = func(session *Session, login string) {
		playerID := session.GetEntityID()
		rtMu.Lock()
		routingTable[playerID] = systemID
		rtMu.Unlock()

		cmd := &protocol.ServerCommand{
			PlayerId: uint64(playerID),
			Type:     protocol.PacketType_C_AUTH_REQUEST,
			Payload:  []byte(login),
		}
		data, _ := proto.Marshal(cmd)
		_ = bus.Publish(fmt.Sprintf("system.%d.input", systemID), data)
	}

	gateway.OnPlayerInput = func(playerID domain.EntityID, pType protocol.PacketType, payload []byte) {
		rtMu.RLock()
		sysID := routingTable[playerID]
		rtMu.RUnlock()

		cmd := &protocol.ServerCommand{
			PlayerId: uint64(playerID),
			Type:     pType,
			Payload:  payload,
		}
		data, _ := proto.Marshal(cmd)
		_ = bus.Publish(fmt.Sprintf("system.%d.input", sysID), data)
	}

	// Subscribe to World Node outputs on Gateway
	_, err = bus.Subscribe("system.*.output", func(msg *messaging.Message) {
		var worldSnap protocol.WorldSnapshot
		if err := proto.Unmarshal(msg.Data, &worldSnap); err != nil {
			return
		}

		sessions := gateway.GetSessions()
		for _, sess := range sessions {
			if sess.GetState() != StateInGame {
				continue
			}
			playerID := sess.GetEntityID()
			rtMu.RLock()
			pSysID := routingTable[playerID]
			rtMu.RUnlock()

			if pSysID == systemID {
				delta, ok := BuildDeltaSnapshotFromWorldState(&worldSnap, sess, 1500.0)
				if ok {
					gateway.SendUnreliable(sess, protocol.PacketType_S_DELTA_SNAPSHOT, delta)
				}
			}
		}
	})
	if err != nil {
		t.Fatalf("Failed to subscribe on gateway: %v", err)
	}

	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("Failed to start gateway: %v", err)
	}
	defer gateway.Stop()

	// 4. Connect a client and test routing
	localAddr := gateway.conn.LocalAddr().String()
	clientConn, err := net.Dial("udp", localAddr)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer clientConn.Close()

	// Auth Request
	authReq, _ := proto.Marshal(&protocol.AuthRequest{
		Login:    "TestPlayer",
		Password: "password",
	})
	authPacket, _ := WrapPacket(1, 0, 0, protocol.PacketType_C_AUTH_REQUEST, authReq)
	_, _ = clientConn.Write(authPacket)

	// Wait for Auth Response
	buf := make([]byte, 2048)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive Auth Response: %v", err)
	}

	respPacket, _ := UnwrapPacket(buf[:n])
	if respPacket.Type != protocol.PacketType_S_AUTH_RESPONSE {
		t.Fatalf("Expected AuthResponse, got %s", respPacket.Type)
	}

	var authResp protocol.AuthResponse
	_ = proto.Unmarshal(respPacket.Payload, &authResp)
	playerID := domain.EntityID(authResp.EntityId)

	// Wait a moment for entity to spawn on worldnode
	time.Sleep(100 * time.Millisecond)

	// Send Move Input
	moveReq, _ := proto.Marshal(&protocol.MoveInput{
		X: 50.0,
		Y: -20.0,
	})
	movePacket, _ := WrapPacket(2, respPacket.Sequence, 1, protocol.PacketType_C_MOVE_INPUT, moveReq)
	_, _ = clientConn.Write(movePacket)

	// Wait for Delta Snapshot from loop tick
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive Delta Snapshot: %v", err)
	}

	snapPacket, _ := UnwrapPacket(buf[:n])
	if snapPacket.Type != protocol.PacketType_S_DELTA_SNAPSHOT {
		t.Fatalf("Expected DeltaSnapshot, got %s", snapPacket.Type)
	}

	var delta protocol.DeltaSnapshot
	_ = proto.Unmarshal(snapPacket.Payload, &delta)

	// Verify that the player entity was included in the snapshot and position updated
	found := false
	for _, ent := range delta.UpdatedEntities {
		if ent.EntityId == uint64(playerID) {
			found = true
			t.Logf("Player coordinates in snapshot: (%.2f, %.2f)", ent.X, ent.Y)
			break
		}
	}

	if !found {
		t.Error("Player not found in delta snapshot")
	}
}

func TestGatewayWorldNode_Corporation(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 1. Shared Messaging Bus
	bus := messaging.NewMockMessageBus()
	defer bus.Close()

	// 2. World Node Setup
	systemID := uint32(1)
	world := ecs.NewWorld()
	grid := spatial.NewHashGrid(1000)
	corpRepo := persistence.NewInMemoryCorporationRepository()

	// Use empty/noop system list or movement system for mock world node
	moveSys := systems.NewMovementSystem(50000, 50000)
	ecsSystems := []ecs.System{moveSys}
	loop := gameloop.NewGameLoop(world, ecsSystems, 20, logger)

	// Subscribe to inputs on World Node
	inputTopic := fmt.Sprintf("system.%d.input", systemID)
	_, err := bus.Subscribe(inputTopic, func(msg *messaging.Message) {
		var cmd protocol.ServerCommand
		if err := proto.Unmarshal(msg.Data, &cmd); err != nil {
			return
		}

		playerID := domain.EntityID(cmd.PlayerId)
		switch cmd.Type {
		case protocol.PacketType_C_AUTH_REQUEST:
			if _, exists := world.GetEntityType(playerID); !exists {
				world.RegisterEntityWithID(playerID, domain.EntityPlayer)
				world.AddComponent(playerID, &domain.Transform{X: 100, Y: 100})
				world.AddComponent(playerID, &domain.Velocity{X: 0, Y: 0})
				world.AddComponent(playerID, &domain.Health{Current: 100, Max: 100})
				world.AddComponent(playerID, &domain.Shield{Current: 50, Max: 50, RegenRate: 1.0})
				world.AddComponent(playerID, &domain.ShipConfig{ShipType: "fighter", MaxSpeed: 100, TurnRate: 1.5})
				world.AddComponent(playerID, &domain.Visibility{Radius: 1500.0, VisibleEntities: make(map[domain.EntityID]struct{})})
				world.AddComponent(playerID, &domain.PlayerData{AccountID: uint64(playerID), Name: string(cmd.Payload), Credits: 1000})
				grid.Insert(playerID, 100, 100)
			}
		case protocol.PacketType_C_CREATE_CORP_REQUEST:
			var req protocol.CreateCorpRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				corp, err := corpRepo.Create(context.Background(), req.Name, cmd.PlayerId)
				var resp protocol.CreateCorpResponse
				if err != nil {
					resp.Success = false
					resp.ErrorMessage = err.Error()
				} else {
					resp.Success = true
					resp.CorpId = corp.ID
					if _, exists := world.GetEntityType(playerID); exists {
						world.AddComponent(playerID, &domain.CorporationMember{
							CorpID: corp.ID,
							Role:   "Owner",
						})
					}
				}
				respPayload, _ := proto.Marshal(&resp)
				packet := &protocol.Packet{
					Type:    protocol.PacketType_S_CREATE_CORP_RESPONSE,
					Payload: respPayload,
				}
				packetData, _ := proto.Marshal(packet)
				_ = bus.Publish(fmt.Sprintf("player.%d.response", cmd.PlayerId), packetData)
			}
		case protocol.PacketType_C_JOIN_CORP_REQUEST:
			var req protocol.JoinCorpRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				err := corpRepo.AddMember(context.Background(), req.CorpId, cmd.PlayerId, "Member")
				if err == nil {
					if _, exists := world.GetEntityType(playerID); exists {
						world.AddComponent(playerID, &domain.CorporationMember{
							CorpID: req.CorpId,
							Role:   "Member",
						})
					}
				}
			}
		}
	})
	if err != nil {
		t.Fatalf("Failed to subscribe on world node: %v", err)
	}

	// Publish outputs (snapshots)
	outputTopic := fmt.Sprintf("system.%d.output", systemID)
	loop.OnSnapshot = func(tick uint64) {
		var entSnaps []*protocol.EntitySnapshot
		allEntities := world.Query(0)
		for _, id := range allEntities {
			snap := BuildEntitySnapshot(world, id)
			if snap != nil {
				entSnaps = append(entSnaps, snap)
			}
		}

		worldSnap := &protocol.WorldSnapshot{
			Tick:     tick,
			Entities: entSnaps,
		}
		data, _ := proto.Marshal(worldSnap)
		_ = bus.Publish(outputTopic, data)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// 3. Gateway Server Setup
	cache := &MockSessionCache{}
	gateway := NewServer("127.0.0.1:0", nil, cache, logger)

	routingTable := make(map[domain.EntityID]uint32)
	var rtMu sync.RWMutex

	gateway.OnPlayerAuth = func(session *Session, login string) {
		playerID := session.GetEntityID()
		rtMu.Lock()
		routingTable[playerID] = systemID
		rtMu.Unlock()

		cmd := &protocol.ServerCommand{
			PlayerId: uint64(playerID),
			Type:     protocol.PacketType_C_AUTH_REQUEST,
			Payload:  []byte(login),
		}
		data, _ := proto.Marshal(cmd)
		_ = bus.Publish(fmt.Sprintf("system.%d.input", systemID), data)
	}

	gateway.OnPlayerInput = func(playerID domain.EntityID, pType protocol.PacketType, payload []byte) {
		rtMu.RLock()
		sysID := routingTable[playerID]
		rtMu.RUnlock()

		cmd := &protocol.ServerCommand{
			PlayerId: uint64(playerID),
			Type:     pType,
			Payload:  payload,
		}
		data, _ := proto.Marshal(cmd)
		_ = bus.Publish(fmt.Sprintf("system.%d.input", sysID), data)
	}

	// Subscribe to outputs on Gateway
	_, err = bus.Subscribe("system.*.output", func(msg *messaging.Message) {
		var worldSnap protocol.WorldSnapshot
		if err := proto.Unmarshal(msg.Data, &worldSnap); err != nil {
			return
		}

		sessions := gateway.GetSessions()
		for _, sess := range sessions {
			if sess.GetState() != StateInGame {
				continue
			}
			playerID := sess.GetEntityID()
			rtMu.RLock()
			pSysID := routingTable[playerID]
			rtMu.RUnlock()

			if pSysID == systemID {
				delta, ok := BuildDeltaSnapshotFromWorldState(&worldSnap, sess, 1500.0)
				if ok {
					gateway.SendUnreliable(sess, protocol.PacketType_S_DELTA_SNAPSHOT, delta)
				}
			}
		}
	})
	if err != nil {
		t.Fatalf("Failed to subscribe system outputs on gateway: %v", err)
	}

	// Subscribe to player.*.response on Gateway
	_, err = bus.Subscribe("player.*.response", func(msg *messaging.Message) {
		parts := strings.Split(msg.Topic, ".")
		if len(parts) < 3 {
			return
		}
		pIDVal, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return
		}
		playerID := domain.EntityID(pIDVal)

		var packet protocol.Packet
		if err := proto.Unmarshal(msg.Data, &packet); err != nil {
			return
		}

		sess := gateway.GetSessionByPlayerID(playerID)
		if sess != nil {
			gateway.SendPacketRaw(sess, packet.Type, packet.Payload, true)
		}
	})
	if err != nil {
		t.Fatalf("Failed to subscribe player response on gateway: %v", err)
	}

	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("Failed to start gateway: %v", err)
	}
	defer gateway.Stop()

	// 4. Connect a client and test routing
	localAddr := gateway.conn.LocalAddr().String()
	clientConn, err := net.Dial("udp", localAddr)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer clientConn.Close()

	// Auth Request
	authReq, _ := proto.Marshal(&protocol.AuthRequest{
		Login:    "CorpFounder",
		Password: "password",
	})
	authPacket, _ := WrapPacket(1, 0, 0, protocol.PacketType_C_AUTH_REQUEST, authReq)
	_, _ = clientConn.Write(authPacket)

	// Wait for Auth Response
	buf := make([]byte, 2048)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive Auth Response: %v", err)
	}

	respPacket, _ := UnwrapPacket(buf[:n])
	if respPacket.Type != protocol.PacketType_S_AUTH_RESPONSE {
		t.Fatalf("Expected AuthResponse, got %s", respPacket.Type)
	}

	var authResp protocol.AuthResponse
	_ = proto.Unmarshal(respPacket.Payload, &authResp)
	playerID := domain.EntityID(authResp.EntityId)

	// Wait a moment for entity to spawn on worldnode
	time.Sleep(100 * time.Millisecond)

	// Send C_CREATE_CORP_REQUEST
	createCorpReq, _ := proto.Marshal(&protocol.CreateCorpRequest{
		Name: "GigaCorp",
	})
	createPacket, _ := WrapPacket(2, respPacket.Sequence, 1, protocol.PacketType_C_CREATE_CORP_REQUEST, createCorpReq)
	_, _ = clientConn.Write(createPacket)

	// Wait for S_CREATE_CORP_RESPONSE
	var corpRespPacket *protocol.Packet
	for {
		_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err = clientConn.Read(buf)
		if err != nil {
			t.Fatalf("Failed to receive CreateCorp Response: %v", err)
		}

		p, err := UnwrapPacket(buf[:n])
		if err != nil {
			t.Fatalf("Failed to unwrap packet: %v", err)
		}
		if p.Type == protocol.PacketType_S_CREATE_CORP_RESPONSE {
			corpRespPacket = p
			break
		}
	}

	var corpResp protocol.CreateCorpResponse
	_ = proto.Unmarshal(corpRespPacket.Payload, &corpResp)

	if !corpResp.Success {
		t.Fatalf("CreateCorp failed: %s", corpResp.ErrorMessage)
	}
	if corpResp.CorpId == 0 {
		t.Fatal("Expected non-zero CorpId")
	}

	// Verify ECS component on player entity
	compVal, foundComp := world.GetComponent(playerID, domain.CorporationMember{})
	if !foundComp {
		t.Fatal("CorporationMember component not found on player entity")
	}
	memberComp := compVal.(*domain.CorporationMember)
	if memberComp.CorpID != corpResp.CorpId {
		t.Errorf("Expected CorpID %d, got %d", corpResp.CorpId, memberComp.CorpID)
	}
	if memberComp.Role != "Owner" {
		t.Errorf("Expected role Owner, got %s", memberComp.Role)
	}

	// Wait for Delta Snapshot from loop tick and verify it contains CorpId
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive Delta Snapshot: %v", err)
	}

	snapPacket, _ := UnwrapPacket(buf[:n])
	// Keep reading if we get ACKs or other packets first
	for snapPacket.Type != protocol.PacketType_S_DELTA_SNAPSHOT {
		n, err = clientConn.Read(buf)
		if err != nil {
			t.Fatalf("Failed to receive Delta Snapshot: %v", err)
		}
		snapPacket, _ = UnwrapPacket(buf[:n])
	}

	var delta protocol.DeltaSnapshot
	_ = proto.Unmarshal(snapPacket.Payload, &delta)

	foundPlayer := false
	for _, ent := range delta.UpdatedEntities {
		if ent.EntityId == uint64(playerID) {
			foundPlayer = true
			if ent.CorpId != corpResp.CorpId {
				t.Errorf("Expected CorpId in snapshot to be %d, got %d", corpResp.CorpId, ent.CorpId)
			}
		}
	}
	if !foundPlayer {
		t.Error("Player not found in delta snapshot after joining corporation")
	}
}

func TestGatewayWorldNode_Production(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 1. Shared Messaging Bus
	bus := messaging.NewMockMessageBus()
	defer bus.Close()

	// 2. World Node Setup
	systemID := uint32(1)
	world := ecs.NewWorld()
	grid := spatial.NewHashGrid(1000)

	// Register RefinerySystem and ShipyardSystem
	refSys := systems.NewRefinerySystem()
	sySys := systems.NewShipyardSystem()
	ecsSystems := []ecs.System{refSys, sySys}
	loop := gameloop.NewGameLoop(world, ecsSystems, 20, logger)

	// Seed one station
	stationID := domain.EntityID(5001)
	world.RegisterEntityWithID(stationID, domain.EntityStation)
	world.AddComponent(stationID, &domain.Transform{X: -300, Y: 200})
	world.AddComponent(stationID, &domain.Refinery{IsActive: false, Yield: 1.0})
	world.AddComponent(stationID, &domain.Shipyard{Queue: []domain.ShipBuildOrder{}})
	// Put 10 Iron and 5 Titanium in cargo to start refining
	world.AddComponent(stationID, &domain.Cargo{
		Items: []domain.ItemInstance{
			{DefinitionID: 1, Quantity: 10, State: "normal"},
			{DefinitionID: 2, Quantity: 5, State: "normal"},
		},
		Capacity: 1000,
	})

	// Subscribe to inputs on World Node
	inputTopic := fmt.Sprintf("system.%d.input", systemID)
	_, err := bus.Subscribe(inputTopic, func(msg *messaging.Message) {
		var cmd protocol.ServerCommand
		if err := proto.Unmarshal(msg.Data, &cmd); err != nil {
			return
		}

		playerID := domain.EntityID(cmd.PlayerId)
		switch cmd.Type {
		case protocol.PacketType_C_AUTH_REQUEST:
			if _, exists := world.GetEntityType(playerID); !exists {
				world.RegisterEntityWithID(playerID, domain.EntityPlayer)
				world.AddComponent(playerID, &domain.Transform{X: 100, Y: 100})
				world.AddComponent(playerID, &domain.Velocity{X: 0, Y: 0})
				world.AddComponent(playerID, &domain.Health{Current: 100, Max: 100})
				world.AddComponent(playerID, &domain.Shield{Current: 50, Max: 50, RegenRate: 1.0})
				world.AddComponent(playerID, &domain.ShipConfig{ShipType: "miner", MaxSpeed: 60, TurnRate: 1.0})
				world.AddComponent(playerID, &domain.Visibility{Radius: 1500.0, VisibleEntities: make(map[domain.EntityID]struct{})})
				world.AddComponent(playerID, &domain.PlayerData{AccountID: uint64(playerID), Name: string(cmd.Payload), Credits: 1000})
				grid.Insert(playerID, 100, 100)
			}
		case protocol.PacketType_C_START_REFINE_REQUEST:
			var req protocol.StartRefineRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				sID := domain.EntityID(req.StationId)
				if refVal, ok := world.GetComponent(sID, domain.Refinery{}); ok {
					ref := refVal.(*domain.Refinery)
					ref.IsActive = true
				}
			}
		case protocol.PacketType_C_BUILD_SHIP_REQUEST:
			var req protocol.BuildShipRequest
			if err := proto.Unmarshal(cmd.Payload, &req); err == nil {
				sID := domain.EntityID(req.StationId)
				syVal, ok1 := world.GetComponent(sID, domain.Shipyard{})
				cargoVal, ok2 := world.GetComponent(sID, domain.Cargo{})
				if ok1 && ok2 {
					sy := syVal.(*domain.Shipyard)
					cargo := cargoVal.(*domain.Cargo)

					var costIron int32 = 0
					var costTitanium int32 = 0
					var totalTime float32 = 1.0 // 1 second build time for fast testing

					if req.ShipType == "fighter" {
						costIron = 2
						costTitanium = 1
					}

					ironPlates := cargo.GetResourceTypeQuantity("IronPlates")
					titaniumPlates := cargo.GetResourceTypeQuantity("TitaniumPlates")

					if ironPlates >= costIron && titaniumPlates >= costTitanium {
						cargo.RemoveResourceTypeQuantity("IronPlates", costIron)
						cargo.RemoveResourceTypeQuantity("TitaniumPlates", costTitanium)

						sy.Queue = append(sy.Queue, domain.ShipBuildOrder{
							ShipType:  req.ShipType,
							Progress:  0.0,
							TotalTime: totalTime,
							OrderedBy: cmd.PlayerId,
						})
					}
				}
			}
		}
	})
	if err != nil {
		t.Fatalf("Failed to subscribe on world node: %v", err)
	}

	// Publish outputs (snapshots)
	outputTopic := fmt.Sprintf("system.%d.output", systemID)
	loop.OnSnapshot = func(tick uint64) {
		var entSnaps []*protocol.EntitySnapshot
		allEntities := world.Query(0)
		for _, id := range allEntities {
			snap := BuildEntitySnapshot(world, id)
			if snap != nil {
				entSnaps = append(entSnaps, snap)
			}
		}

		worldSnap := &protocol.WorldSnapshot{
			Tick:     tick,
			Entities: entSnaps,
		}
		data, _ := proto.Marshal(worldSnap)
		_ = bus.Publish(outputTopic, data)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// 3. Gateway Server Setup
	cache := &MockSessionCache{}
	gateway := NewServer("127.0.0.1:0", nil, cache, logger)

	routingTable := make(map[domain.EntityID]uint32)
	var rtMu sync.RWMutex

	gateway.OnPlayerAuth = func(session *Session, login string) {
		playerID := session.GetEntityID()
		rtMu.Lock()
		routingTable[playerID] = systemID
		rtMu.Unlock()

		cmd := &protocol.ServerCommand{
			PlayerId: uint64(playerID),
			Type:     protocol.PacketType_C_AUTH_REQUEST,
			Payload:  []byte(login),
		}
		data, _ := proto.Marshal(cmd)
		_ = bus.Publish(fmt.Sprintf("system.%d.input", systemID), data)
	}

	gateway.OnPlayerInput = func(playerID domain.EntityID, pType protocol.PacketType, payload []byte) {
		rtMu.RLock()
		sysID := routingTable[playerID]
		rtMu.RUnlock()

		cmd := &protocol.ServerCommand{
			PlayerId: uint64(playerID),
			Type:     pType,
			Payload:  payload,
		}
		data, _ := proto.Marshal(cmd)
		_ = bus.Publish(fmt.Sprintf("system.%d.input", sysID), data)
	}

	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("Failed to start gateway: %v", err)
	}
	defer gateway.Stop()

	// 4. Connect a client
	localAddr := gateway.conn.LocalAddr().String()
	clientConn, err := net.Dial("udp", localAddr)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer clientConn.Close()

	// Auth Request
	authReq, _ := proto.Marshal(&protocol.AuthRequest{
		Login:    "Industrialist",
		Password: "password",
	})
	authPacket, _ := WrapPacket(1, 0, 0, protocol.PacketType_C_AUTH_REQUEST, authReq)
	_, _ = clientConn.Write(authPacket)

	// Wait for Auth Response
	buf := make([]byte, 2048)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive Auth Response: %v", err)
	}

	respPacket, _ := UnwrapPacket(buf[:n])
	var authResp protocol.AuthResponse
	_ = proto.Unmarshal(respPacket.Payload, &authResp)
	playerID := domain.EntityID(authResp.EntityId)

	// Wait a moment for entity to spawn
	time.Sleep(100 * time.Millisecond)

	// 5. Send C_START_REFINE_REQUEST
	refineReq, _ := proto.Marshal(&protocol.StartRefineRequest{
		StationId: uint64(stationID),
	})
	refinePacket, _ := WrapPacket(2, respPacket.Sequence, 1, protocol.PacketType_C_START_REFINE_REQUEST, refineReq)
	_, _ = clientConn.Write(refinePacket)

	// Let simulation run for 2.5 seconds (which triggers refinery several ticks)
	// Refinery processes 2 ores per tick. 10 Iron -> 5 IronPlates. 5 Titanium -> 2 TitaniumPlates.
	time.Sleep(2500 * time.Millisecond)

	// Check if refinery processed
	cargoVal, _ := world.GetComponent(stationID, domain.Cargo{})
	cargo := cargoVal.(*domain.Cargo)
	ironPlates := cargo.GetResourceTypeQuantity("IronPlates")
	titaniumPlates := cargo.GetResourceTypeQuantity("TitaniumPlates")

	if ironPlates < 2 || titaniumPlates < 1 {
		t.Errorf("Expected plates to be refined, got IronPlates=%d, TitaniumPlates=%d", ironPlates, titaniumPlates)
	}

	// 6. Send C_BUILD_SHIP_REQUEST (Fighter costs 5 IronPlates, 2 TitaniumPlates)
	buildReq, _ := proto.Marshal(&protocol.BuildShipRequest{
		StationId: uint64(stationID),
		ShipType:  "fighter",
	})
	buildPacket, _ := WrapPacket(3, respPacket.Sequence, 1, protocol.PacketType_C_BUILD_SHIP_REQUEST, buildReq)
	_, _ = clientConn.Write(buildPacket)

	// Let simulation run for 1.5 seconds (which completes the 1-second build order)
	time.Sleep(1500 * time.Millisecond)

	// Check if player's ship configuration has been updated
	cfgVal, _ := world.GetComponent(playerID, domain.ShipConfig{})
	cfg := cfgVal.(*domain.ShipConfig)

	if cfg.ShipType != "fighter" {
		t.Errorf("Expected ship type to be fighter, got %s", cfg.ShipType)
	}
	if cfg.MaxSpeed != 120 {
		t.Errorf("Expected max speed to be 120, got %f", cfg.MaxSpeed)
	}
}
