package redis

import (
	"context"

	"github.com/gabkaclassic/gorbage/internal/config"
	"github.com/redis/go-redis/v9"
)

func NewRedisConnection(ctx context.Context, cfg config.Redis) (*redis.Client, error) {

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Host,
		Password: cfg.Password,
		Username: cfg.Username,
		DB:       cfg.DB,
	})

	_, err := client.Ping(ctx).Result()

	if err != nil {
		return nil, err
	}

	return client, err
}
