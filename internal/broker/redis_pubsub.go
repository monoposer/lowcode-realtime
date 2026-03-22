package broker

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisPubSubBroker is a MessageBroker implementation backed by Redis Pub/Sub.
// It focuses on simplicity and low latency, without persistence guarantees.
type RedisPubSubBroker struct {
	client *redis.Client
}

// RedisPubSubConfig contains basic Redis settings for the broker.
type RedisPubSubConfig struct {
	Addr     string
	Password string
	DB       int
}

// NewRedisPubSubBroker creates a new RedisPubSubBroker.
func NewRedisPubSubBroker(cfg RedisPubSubConfig) *RedisPubSubBroker {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return &RedisPubSubBroker{client: rdb}
}

func (b *RedisPubSubBroker) PublishBroadcast(ctx context.Context, channel string, data []byte) error {
	return b.client.Publish(ctx, "broadcast."+channel, data).Err()
}

func (b *RedisPubSubBroker) SubscribeBroadcast(ctx context.Context, channelPattern string) (<-chan Message, error) {
	ps := b.client.PSubscribe(ctx, "broadcast."+channelPattern)
	out := make(chan Message)

	go func() {
		defer close(out)
		for {
			msg, err := ps.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			out <- Message{
				Channel:   trimPrefix(msg.Channel, "broadcast."),
				Data:      []byte(msg.Payload),
				Timestamp: time.Now().UnixMilli(),
			}
		}
	}()

	return out, nil
}

func (b *RedisPubSubBroker) PublishPresenceEvent(ctx context.Context, channel string, data []byte) error {
	return b.client.Publish(ctx, "presence."+channel, data).Err()
}

func (b *RedisPubSubBroker) SubscribePresenceEvent(ctx context.Context, channelPattern string) (<-chan Message, error) {
	ps := b.client.PSubscribe(ctx, "presence."+channelPattern)
	out := make(chan Message)

	go func() {
		defer close(out)
		for {
			msg, err := ps.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			out <- Message{
				Channel:   trimPrefix(msg.Channel, "presence."),
				Data:      []byte(msg.Payload),
				Timestamp: time.Now().UnixMilli(),
			}
		}
	}()

	return out, nil
}

func (b *RedisPubSubBroker) Close() error {
	return b.client.Close()
}

// trimPrefix is a small helper to strip a prefix from a string.
func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

// Register the Redis implementation as the default broadcast MQ plugin.
func init() {
	RegisterBroadcastImpl("redis", func(cfg map[string]string) (BroadcastImpl, error) {
		db := 0
		if v, ok := cfg["db"]; ok {
			if parsed, err := parseInt(v); err == nil {
				db = parsed
			}
		}

		return NewRedisPubSubBroker(RedisPubSubConfig{
			Addr:     cfg["addr"],
			Password: cfg["password"],
			DB:       db,
		}), nil
	})
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}


