package broker

import (
	"context"
	"fmt"
)

// BroadcastImpl is the plugin surface for broadcast-only MQ backends.
type BroadcastImpl interface {
	PublishBroadcast(ctx context.Context, channel string, data []byte) error
	SubscribeBroadcast(ctx context.Context, channelPattern string) (<-chan Message, error)
	Close() error
}

// PresenceImpl is the plugin surface for presence events; currently backed by Redis.
type PresenceImpl interface {
	PublishPresenceEvent(ctx context.Context, channel string, data []byte) error
	SubscribePresenceEvent(ctx context.Context, channelPattern string) (<-chan Message, error)
	Close() error
}

// CompositeBroker combines a broadcast implementation and a presence implementation
// and exposes the unified MessageBroker interface.
type CompositeBroker struct {
	broadcast BroadcastImpl
	presence  PresenceImpl
}

func (c *CompositeBroker) PublishBroadcast(ctx context.Context, channel string, data []byte) error {
	return c.broadcast.PublishBroadcast(ctx, channel, data)
}

func (c *CompositeBroker) SubscribeBroadcast(ctx context.Context, channelPattern string) (<-chan Message, error) {
	return c.broadcast.SubscribeBroadcast(ctx, channelPattern)
}

func (c *CompositeBroker) PublishPresenceEvent(ctx context.Context, channel string, data []byte) error {
	return c.presence.PublishPresenceEvent(ctx, channel, data)
}

func (c *CompositeBroker) SubscribePresenceEvent(ctx context.Context, channelPattern string) (<-chan Message, error) {
	return c.presence.SubscribePresenceEvent(ctx, channelPattern)
}

func (c *CompositeBroker) Close() error {
	_ = c.broadcast.Close()
	_ = c.presence.Close()
	return nil
}

// BroadcastFactory builds a BroadcastImpl from string-keyed config.
type BroadcastFactory func(cfg map[string]string) (BroadcastImpl, error)

var broadcastRegistry = map[string]BroadcastFactory{}

// RegisterBroadcastImpl registers a broadcast MQ plugin (e.g. "redis", "kafka", "rabbitmq").
func RegisterBroadcastImpl(name string, factory BroadcastFactory) {
	if name == "" || factory == nil {
		return
	}
	broadcastRegistry[name] = factory
}

// BrokerConfig holds everything needed to construct a MessageBroker.
type BrokerConfig struct {
	// BroadcastType selects the MQ backend for broadcast, e.g. "redis", "kafka", "rabbitmq".
	BroadcastType string
	// BroadcastCfg is passed to the selected BroadcastImpl (plugin-specific keys).
	BroadcastCfg map[string]string
	// PresenceCfg configures Redis for presence pub/sub.
	PresenceCfg RedisPubSubConfig
}

// NewMessageBroker builds a composite MessageBroker:
// - Presence always uses RedisPubSubBroker
// - Broadcast is resolved from BroadcastType via the registry
func NewMessageBroker(cfg BrokerConfig) (MessageBroker, error) {
	if cfg.BroadcastType == "" {
		cfg.BroadcastType = "redis"
	}

	factory, ok := broadcastRegistry[cfg.BroadcastType]
	if !ok {
		return nil, fmt.Errorf("unsupported broadcast mq type: %s", cfg.BroadcastType)
	}

	broadcastImpl, err := factory(cfg.BroadcastCfg)
	if err != nil {
		return nil, err
	}

	presenceImpl := NewRedisPubSubBroker(cfg.PresenceCfg)

	return &CompositeBroker{
		broadcast: broadcastImpl,
		presence:  presenceImpl,
	}, nil
}
