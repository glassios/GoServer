package messaging

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMockMessageBus_PubSub(t *testing.T) {
	bus := NewMockMessageBus()
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	received := make(map[string]string)
	var mu sync.Mutex

	// Subscribe to a specific topic
	_, err := bus.Subscribe("test.topic", func(msg *Message) {
		mu.Lock()
		received["specific"] = string(msg.Data)
		mu.Unlock()
		wg.Done()
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Subscribe using wildcard
	_, err = bus.Subscribe("test.*", func(msg *Message) {
		mu.Lock()
		received["wildcard"] = string(msg.Data)
		mu.Unlock()
		wg.Done()
	})
	if err != nil {
		t.Fatalf("Subscribe wildcard failed: %v", err)
	}

	err = bus.Publish("test.topic", []byte("hello"))
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Wait with timeout
	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()

	select {
	case <-c:
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for messages")
	}

	mu.Lock()
	defer mu.Unlock()
	if received["specific"] != "hello" {
		t.Errorf("Expected 'hello', got '%s'", received["specific"])
	}
	if received["wildcard"] != "hello" {
		t.Errorf("Expected 'hello' via wildcard, got '%s'", received["wildcard"])
	}
}

func TestMockMessageBus_RequestReply(t *testing.T) {
	bus := NewMockMessageBus()
	defer bus.Close()

	_, err := bus.Reply("test.request", func(data []byte) ([]byte, error) {
		return []byte(string(data) + " world"), nil
	})
	if err != nil {
		t.Fatalf("Reply failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, err := bus.Request(ctx, "test.request", []byte("hello"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if string(res) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(res))
	}
}

func TestNATSMessageBus_Integration(t *testing.T) {
	// Try to connect to local NATS, skip if not running
	bus, err := NewNATSMessageBus("nats://localhost:4222")
	if err != nil {
		t.Skip("NATS server is not running on localhost:4222, skipping integration test")
	}
	defer bus.Close()

	_, err = bus.Reply("test.request", func(data []byte) ([]byte, error) {
		return []byte(string(data) + " world"), nil
	})
	if err != nil {
		t.Fatalf("Reply failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	res, err := bus.Request(ctx, "test.request", []byte("hello"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if string(res) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(res))
	}
}
