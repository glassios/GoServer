package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Home/galaxy-mmo/internal/domain"
)

type RedisSessionCache struct {
	client *redis.Client
}

func NewRedisSessionCache(client *redis.Client) *RedisSessionCache {
	return &RedisSessionCache{
		client: client,
	}
}

func (c *RedisSessionCache) Set(ctx context.Context, sessionID string, accountID uint64, ttl time.Duration) error {
	key := fmt.Sprintf("session:%s", sessionID)
	val := strconv.FormatUint(accountID, 10)
	err := c.client.Set(ctx, key, val, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to save session to redis: %w", err)
	}
	return nil
}

func (c *RedisSessionCache) Get(ctx context.Context, sessionID string) (uint64, error) {
	key := fmt.Sprintf("session:%s", sessionID)
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, domain.ErrSessionExpired
	} else if err != nil {
		return 0, fmt.Errorf("failed to get session from redis: %w", err)
	}

	accountID, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse account ID from redis value: %w", err)
	}

	return accountID, nil
}

func (c *RedisSessionCache) Delete(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete session from redis: %w", err)
	}
	return nil
}
