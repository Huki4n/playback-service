package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

type Config struct {
	DSN         string        `mapstructure:"dsn"`
	MaxConns    int32         `mapstructure:"max_conns"`
	MinConns    int32         `mapstructure:"min_conns"`
	MaxConnLife time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdle time.Duration `mapstructure:"max_conn_idle_time"`
	ConnTimeout time.Duration `mapstructure:"connect_timeout"`
}

func New(lc fx.Lifecycle, cfg Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse postgres DSN: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLife > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLife
	}
	if cfg.MaxConnIdle > 0 {
		poolCfg.MaxConnIdleTime = cfg.MaxConnIdle
	}

	poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	logger.Info("postgres connected", "dsn_host", poolCfg.ConnConfig.Host)

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			logger.Info("closing postgres pool")
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

// HealthCheck pings the database and returns an error if unreachable.
func HealthCheck(ctx context.Context, pool *pgxpool.Pool) error {
	return pool.Ping(ctx)
}
