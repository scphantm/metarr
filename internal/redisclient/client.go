// Package redisclient constructs the shared Redis connection used for both
// the pub/sub message queue and the stream-based event bus.
package redisclient

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func New(ctx context.Context, uri string) (*redis.Client, error) {
	opts, err := redis.ParseURL(uri)
	if err != nil {
		return nil, fmt.Errorf("redisclient: parsing redis uri: %w", err)
	}

	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return client, nil
}
