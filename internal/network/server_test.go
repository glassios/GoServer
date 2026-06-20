package network

import (
	"context"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/Home/galaxy-mmo/internal/ecs"
	"github.com/Home/galaxy-mmo/internal/gameloop"
	"github.com/Home/galaxy-mmo/pkg/protocol"
)

type MockSessionCache struct{}

func (c *MockSessionCache) Set(ctx context.Context, sessionID string, accountID uint64, ttl time.Duration) error {
	return nil
}
func (c *MockSessionCache) Get(ctx context.Context, sessionID string) (uint64, error) {
	return 1, nil
}
func (c *MockSessionCache) Delete(ctx context.Context, sessionID string) error {
	return nil
}

func TestServer_AuthAndPing(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	world := ecs.NewWorld()
	loop := gameloop.NewGameLoop(world, []ecs.System{}, 20, logger)

	// In Go, loop.Run blocks, so we don't start the loop itself,
	// but we can verify that commands are queued in its queue.
	cache := &MockSessionCache{}
	server := NewServer("127.0.0.1:0", loop, cache, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	// Get the bound port
	localAddr := server.conn.LocalAddr().String()

	// Client Socket
	clientConn, err := net.Dial("udp", localAddr)
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer clientConn.Close()

	// 1. Send Auth Request
	authReqPayload, _ := proto.Marshal(&protocol.AuthRequest{
		Login:    "Player1",
		Password: "password",
	})
	authPacketData, _ := WrapPacket(1, 0, 0, protocol.PacketType_C_AUTH_REQUEST, authReqPayload)

	if _, err := clientConn.Write(authPacketData); err != nil {
		t.Fatalf("failed to send auth request: %v", err)
	}

	// Read Response
	buf := make([]byte, 2048)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read auth response: %v", err)
	}

	responsePacket, err := UnwrapPacket(buf[:n])
	if err != nil {
		t.Fatalf("failed to unwrap response packet: %v", err)
	}

	if responsePacket.Type != protocol.PacketType_S_AUTH_RESPONSE {
		t.Errorf("expected PacketType_S_AUTH_RESPONSE, got %v", responsePacket.Type)
	}

	var authResp protocol.AuthResponse
	if err := proto.Unmarshal(responsePacket.Payload, &authResp); err != nil {
		t.Fatalf("failed to parse auth response payload: %v", err)
	}

	if !authResp.Success {
		t.Error("expected auth response Success to be true")
	}

	// 2. Send Ping
	pingPayload, _ := proto.Marshal(&protocol.Ping{
		Timestamp: 12345.67,
	})
	pingPacketData, _ := WrapPacket(2, responsePacket.Sequence, 1, protocol.PacketType_C_PING, pingPayload)

	if _, err := clientConn.Write(pingPacketData); err != nil {
		t.Fatalf("failed to send ping: %v", err)
	}

	// Read Pong
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read pong: %v", err)
	}

	pongPacket, err := UnwrapPacket(buf[:n])
	if err != nil {
		t.Fatalf("failed to unwrap pong packet: %v", err)
	}

	if pongPacket.Type != protocol.PacketType_S_PONG {
		t.Errorf("expected PacketType_S_PONG, got %v", pongPacket.Type)
	}

	var pong protocol.Pong
	if err := proto.Unmarshal(pongPacket.Payload, &pong); err != nil {
		t.Fatalf("failed to parse pong payload: %v", err)
	}

	if pong.Timestamp != 12345.67 {
		t.Errorf("expected timestamp 12345.67, got %f", pong.Timestamp)
	}
}

func TestServer_RateLimiter(t *testing.T) {
	logger := zap.NewNop() // mute logs for flood test
	world := ecs.NewWorld()
	loop := gameloop.NewGameLoop(world, []ecs.System{}, 20, logger)

	cache := &MockSessionCache{}
	server := NewServer("127.0.0.1:0", loop, cache, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	localAddr := server.conn.LocalAddr().String()
	clientConn, err := net.Dial("udp", localAddr)
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer clientConn.Close()

	// Send 100 ping packets instantly
	pingPayload, _ := proto.Marshal(&protocol.Ping{Timestamp: 1.0})
	pingPacketData, _ := WrapPacket(1, 0, 0, protocol.PacketType_C_PING, pingPayload)

	for i := 0; i < 100; i++ {
		_, _ = clientConn.Write(pingPacketData)
	}

	// Count responses
	responsesCount := 0
	buf := make([]byte, 2048)
	deadline := time.Now().Add(500 * time.Millisecond)

	for {
		_ = clientConn.SetReadDeadline(deadline)
		_, err := clientConn.Read(buf)
		if err != nil {
			break // timeout or socket closed
		}
		responsesCount++
	}

	// Since limit is 60 pps, and token bucket initializes with 60 capacity,
	// some packets must have been dropped.
	if responsesCount >= 100 {
		t.Errorf("expected rate limiter to drop some packets, but received all %d responses", responsesCount)
	}
	t.Logf("Rate limiter allowed %d of 100 packets", responsesCount)
}
