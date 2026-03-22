package presence

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Meta represents presence metadata stored in Redis.
type Meta struct {
	UserID    string                 `json:"user_id"`
	Meta      map[string]interface{} `json:"meta"`
	UpdatedAt int64                  `json:"updated_at"`
}

// Store defines operations on the presence store.
type Store interface {
	Upsert(ctx context.Context, channelID, userID string, meta map[string]interface{}, ttl time.Duration) (Meta, error)
	Remove(ctx context.Context, channelID, userID string) error
	List(ctx context.Context, channelID string) ([]Meta, error)
}

// RedisStore is a presence store based on Redis Hash + TTL.
type RedisStore struct {
	client *redis.Client
	prefix string
}

// NewRedisStore creates a new Redis-backed presence store.
func NewRedisStore(client *redis.Client, prefix string) *RedisStore {
	if prefix == "" {
		prefix = "presence:"
	}
	return &RedisStore{
		client: client,
		prefix: prefix,
	}
}

func (s *RedisStore) key(channelID string) string {
	return s.prefix + channelID
}

func (s *RedisStore) Upsert(ctx context.Context, channelID, userID string, meta map[string]interface{}, ttl time.Duration) (Meta, error) {
	now := time.Now().Unix()
	m := Meta{
		UserID:    userID,
		Meta:      meta,
		UpdatedAt: now,
	}
	data, err := json.Marshal(m)
	if err != nil {
		return Meta{}, err
	}

	key := s.key(channelID)

	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, key, userID, data)
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return Meta{}, err
	}

	return m, nil
}

func (s *RedisStore) Remove(ctx context.Context, channelID, userID string) error {
	key := s.key(channelID)
	return s.client.HDel(ctx, key, userID).Err()
}

func (s *RedisStore) List(ctx context.Context, channelID string) ([]Meta, error) {
	key := s.key(channelID)
	res, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Meta, 0, len(res))
	for _, v := range res {
		var m Meta
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

