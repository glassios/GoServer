package redis

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type OnlineTracker struct {
	client *redis.Client
}

func NewOnlineTracker(client *redis.Client) *OnlineTracker {
	return &OnlineTracker{
		client: client,
	}
}

func (t *OnlineTracker) TrackOnline(ctx context.Context, accountID uint64) error {
	val := strconv.FormatUint(accountID, 10)
	err := t.client.SAdd(ctx, "online_players", val).Err()
	if err != nil {
		return fmt.Errorf("failed to add player to online set: %w", err)
	}
	return nil
}

func (t *OnlineTracker) TrackOffline(ctx context.Context, accountID uint64) error {
	val := strconv.FormatUint(accountID, 10)
	err := t.client.SRem(ctx, "online_players", val).Err()
	if err != nil {
		return fmt.Errorf("failed to remove player from online set: %w", err)
	}
	return nil
}

func (t *OnlineTracker) GetOnlineCount(ctx context.Context) (int64, error) {
	count, err := t.client.SCard(ctx, "online_players").Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get online players count: %w", err)
	}
	return count, nil
}
