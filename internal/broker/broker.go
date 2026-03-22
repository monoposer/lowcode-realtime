package broker

import "context"

// Message represents a message delivered via the message broker.
type Message struct {
	Channel   string
	Data      []byte
	Timestamp int64
}

// MessageBroker defines the abstraction for the MQ layer.
// Different implementations (Redis, Kafka, RabbitMQ, etc.) should satisfy this interface.
type MessageBroker interface {
	// PublishBroadcast publishes a broadcast message to a given channel.
	PublishBroadcast(ctx context.Context, channel string, data []byte) error

	// SubscribeBroadcast subscribes to broadcast messages for a given channel pattern.
	// The returned channel is closed when the subscription is terminated.
	SubscribeBroadcast(ctx context.Context, channelPattern string) (<-chan Message, error)

	// PublishPresenceEvent publishes a presence event to a given channel.
	PublishPresenceEvent(ctx context.Context, channel string, data []byte) error

	// SubscribePresenceEvent subscribes to presence events for a given channel pattern.
	SubscribePresenceEvent(ctx context.Context, channelPattern string) (<-chan Message, error)

	// Close releases all underlying resources.
	Close() error
}

