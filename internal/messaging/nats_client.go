package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type natsSubscription struct {
	sub *nats.Subscription
}

func (s *natsSubscription) Unsubscribe() error {
	return s.sub.Unsubscribe()
}

// NATSMessageBus implements MessageBus interface using NATS broker.
type NATSMessageBus struct {
	nc *nats.Conn
}

// NewNATSMessageBus connects to a NATS server and returns a client wrapper.
func NewNATSMessageBus(url string) (*NATSMessageBus, error) {
	nc, err := nats.Connect(url,
		nats.Name("Galaxy-MMO-Server"),
		nats.Timeout(5*time.Second),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(5),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	return &NATSMessageBus{nc: nc}, nil
}

// Publish sends a message to the specified topic.
func (b *NATSMessageBus) Publish(topic string, data []byte) error {
	return b.nc.Publish(topic, data)
}

// Subscribe listens to messages on a topic.
func (b *NATSMessageBus) Subscribe(topic string, handler Handler) (Subscription, error) {
	sub, err := b.nc.Subscribe(topic, func(msg *nats.Msg) {
		handler(&Message{
			Topic: msg.Subject,
			Data:  msg.Data,
			Reply: msg.Reply,
		})
	})
	if err != nil {
		return nil, err
	}
	return &natsSubscription{sub: sub}, nil
}

// Request sends a request on a topic and blocks waiting for a reply or context expiration.
func (b *NATSMessageBus) Request(ctx context.Context, topic string, data []byte) ([]byte, error) {
	msg, err := b.nc.RequestWithContext(ctx, topic, data)
	if err != nil {
		return nil, err
	}
	return msg.Data, nil
}

// Reply listens to requests on a topic and replies using the response returned by the handler.
func (b *NATSMessageBus) Reply(topic string, handler func(data []byte) ([]byte, error)) (Subscription, error) {
	sub, err := b.nc.Subscribe(topic, func(msg *nats.Msg) {
		res, err := handler(msg.Data)
		if err != nil {
			return
		}
		_ = msg.Respond(res)
	})
	if err != nil {
		return nil, err
	}
	return &natsSubscription{sub: sub}, nil
}

// Close closes the NATS connection.
func (b *NATSMessageBus) Close() error {
	b.nc.Close()
	return nil
}
