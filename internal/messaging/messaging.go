package messaging

import (
	"context"
)

// Message represents a generic message sent over the messaging bus.
type Message struct {
	Topic string
	Data  []byte
	Reply string // For request-response pattern
}

// Handler is a callback function for processing incoming messages.
type Handler func(msg *Message)

// Subscription represents an active subscription that can be cancelled.
type Subscription interface {
	Unsubscribe() error
}

// MessageBus defines the interface for our pub/sub and request/reply messaging system.
type MessageBus interface {
	Publish(topic string, data []byte) error
	Subscribe(topic string, handler Handler) (Subscription, error)
	Request(ctx context.Context, topic string, data []byte) ([]byte, error)
	Reply(topic string, handler func(data []byte) ([]byte, error)) (Subscription, error)
	Close() error
}
