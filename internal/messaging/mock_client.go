package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type mockSubscription struct {
	id    int
	topic string
	bus   *MockMessageBus
}

func (s *mockSubscription) Unsubscribe() error {
	s.bus.unsubscribe(s.id)
	return nil
}

type mockSub struct {
	id      int
	topic   string
	handler Handler
}

// MockMessageBus is an in-memory implementation of MessageBus for local/offline execution.
type MockMessageBus struct {
	mu     sync.RWMutex
	subs   map[int]*mockSub
	nextID int
	closed bool
}

// NewMockMessageBus creates a new in-memory message bus.
func NewMockMessageBus() *MockMessageBus {
	return &MockMessageBus{
		subs: make(map[int]*mockSub),
	}
}

// Publish sends data to a specific topic.
func (b *MockMessageBus) Publish(topic string, data []byte) error {
	return b.PublishWithReply(topic, data, "")
}

// PublishWithReply sends data to a specific topic, specifying a reply topic.
func (b *MockMessageBus) PublishWithReply(topic string, data []byte, reply string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return errors.New("messaging: bus closed")
	}

	msg := &Message{
		Topic: topic,
		Data:  data,
		Reply: reply,
	}

	for _, sub := range b.subs {
		if matchTopic(sub.topic, topic) {
			// Run handler concurrently to match async NATS behavior
			go sub.handler(msg)
		}
	}

	return nil
}

// Subscribe listens to messages on a topic. Supports '*' and '>' wildcards.
func (b *MockMessageBus) Subscribe(topic string, handler Handler) (Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, errors.New("messaging: bus closed")
	}

	id := b.nextID
	b.nextID++

	b.subs[id] = &mockSub{
		id:      id,
		topic:   topic,
		handler: handler,
	}

	return &mockSubscription{
		id:    id,
		topic: topic,
		bus:   b,
	}, nil
}

// Request sends a request on a topic and blocks waiting for a reply.
func (b *MockMessageBus) Request(ctx context.Context, topic string, data []byte) ([]byte, error) {
	replyInbox := fmt.Sprintf("_INBOX.%d", time.Now().UnixNano())

	ch := make(chan []byte, 1)
	sub, err := b.Subscribe(replyInbox, func(msg *Message) {
		select {
		case ch <- msg.Data:
		default:
		}
	})
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()

	if err := b.PublishWithReply(topic, data, replyInbox); err != nil {
		return nil, err
	}

	select {
	case res := <-ch:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Reply registers a request handler on a topic and responds to the sender.
func (b *MockMessageBus) Reply(topic string, handler func(data []byte) ([]byte, error)) (Subscription, error) {
	return b.Subscribe(topic, func(msg *Message) {
		if msg.Reply == "" {
			return
		}
		res, err := handler(msg.Data)
		if err != nil {
			return
		}
		_ = b.Publish(msg.Reply, res)
	})
}

func (b *MockMessageBus) unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, id)
}

// Close closes the message bus and clears subscriptions.
func (b *MockMessageBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.subs = make(map[int]*mockSub)
	return nil
}

func matchTopic(pattern, topic string) bool {
	if pattern == ">" {
		return true
	}
	pTokens := strings.Split(pattern, ".")
	tTokens := strings.Split(topic, ".")
	for i, pToken := range pTokens {
		if pToken == ">" {
			return true
		}
		if i >= len(tTokens) {
			return false
		}
		if pToken != "*" && pToken != tTokens[i] {
			return false
		}
	}
	return len(pTokens) == len(tTokens)
}
