package protocol

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestProtocol_SerializationRoundtrip(t *testing.T) {
	// Create an AuthResponse message
	resp := &AuthResponse{
		Success:      true,
		SessionToken: "test-token-12345",
		EntityId:     42,
		ErrorMessage: "",
	}

	// Marshal to bytes
	data, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal AuthResponse: %v", err)
	}

	// Unmarshal back
	var decoded AuthResponse
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal AuthResponse: %v", err)
	}

	// Verify values
	if !decoded.Success {
		t.Error("expected Success to be true")
	}
	if decoded.SessionToken != "test-token-12345" {
		t.Errorf("expected SessionToken 'test-token-12345', got %s", decoded.SessionToken)
	}
	if decoded.EntityId != 42 {
		t.Errorf("expected EntityId 42, got %d", decoded.EntityId)
	}
}

func BenchmarkProtocol_Marshal_100_Entities(b *testing.B) {
	// Setup WorldSnapshot with 100 entities
	snapshot := &WorldSnapshot{
		Tick:     1234,
		Entities: make([]*EntitySnapshot, 100),
	}

	for i := 0; i < 100; i++ {
		snapshot.Entities[i] = &EntitySnapshot{
			EntityId:   uint64(i),
			EntityType: 1, // Player
			X:          float32(i * 10),
			Y:          float32(i * 10),
			Rotation:   0.5,
			Vx:         1.0,
			Vy:         2.0,
			Hp:         100,
			MaxHp:      100,
			Shield:     50,
			MaxShield:  50,
			Name:       fmt.Sprintf("Player_%d", i),
			FactionId:  2,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := proto.Marshal(snapshot)
		if err != nil {
			b.Fatalf("marshal failed: %v", err)
		}
		_ = data
	}
}
