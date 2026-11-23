package redis

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

func NewRedis(lc fx.Lifecycle) *redis.Client {
	client := redis.NewClient(
		&redis.Options{

			Addr: fmt.Sprintf("%s:%s", os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT")),
		},
	)

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return client.Close()
		},
	})

	return client
}
