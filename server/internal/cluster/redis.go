package cluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisBus implements Bus on a Redis server (or a Redis-compatible service
// such as Valkey/Dragonfly). Pub/sub carries RPC and frames; plain keys hold
// ownership, liveness and invite codes.
type RedisBus struct {
	c *redis.Client
}

// OpenRedis connects using a redis:// URL and verifies the server answers.
func OpenRedis(ctx context.Context, url string) (*RedisBus, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis url: %w", err)
	}
	c := redis.NewClient(opt)
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.Ping(pctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisBus{c: c}, nil
}

func (b *RedisBus) Publish(ctx context.Context, channel string, payload []byte) error {
	return b.c.Publish(ctx, channel, payload).Err()
}

func (b *RedisBus) Subscribe(ctx context.Context, channel string, fn func([]byte)) (func(), error) {
	ps := b.c.Subscribe(ctx, channel)
	// Wait for the subscription to be confirmed so callers can publish to
	// themselves right away.
	if _, err := ps.Receive(ctx); err != nil {
		_ = ps.Close()
		return nil, err
	}
	go func() {
		for msg := range ps.Channel() {
			fn([]byte(msg.Payload))
		}
	}()
	return func() { _ = ps.Close() }, nil
}

func (b *RedisBus) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return b.c.Set(ctx, key, value, ttl).Err()
}

func (b *RedisBus) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return b.c.SetNX(ctx, key, value, ttl).Result()
}

func (b *RedisBus) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := b.c.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (b *RedisBus) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return b.c.Del(ctx, keys...).Err()
}

func (b *RedisBus) Close() error { return b.c.Close() }
