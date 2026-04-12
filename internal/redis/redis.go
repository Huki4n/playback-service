package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

type Config struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

func New(lc fx.Lifecycle, cfg Config, logger *slog.Logger) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}

	client := redis.NewClient(opts)

	if err := redisotel.InstrumentTracing(client); err != nil {
		return nil, fmt.Errorf("redis otel tracing: %w", err)
	}
	if err := redisotel.InstrumentMetrics(client); err != nil {
		return nil, fmt.Errorf("redis otel metrics: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	logger.Info("redis connected", "addr", cfg.Addr)

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			logger.Info("closing redis connection")
			return client.Close()
		},
	})

	return client, nil
}

// HealthCheck pings Redis and returns an error if unreachable.
func HealthCheck(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}
