package main

import (
	"log/slog"
	"service/internal/config"
	"service/internal/handler"
	"service/internal/kafka"
	"service/internal/logger"
	"service/internal/postgres"
	"service/internal/server"
	"service/internal/session"
	"service/internal/tracing"

	"go.uber.org/fx"

	goredis "service/internal/redis"
)

// @title           Playback Session Service API
// @version         1.0
// @description     Сервис управления сессиями воспроизведения музыкального стримингового сервиса.
// @host            localhost:8080
// @BasePath        /api/v1
func main() {
	cfg := config.Load()

	fx.New(
		fx.Supply(cfg),
		fx.Provide(
			logger.New,
			handler.New,

			// Infrastructure
			func(c config.Config) postgres.Config { return c.Postgres },
			func(c config.Config) goredis.Config { return c.Redis },
			func(c config.Config) kafka.ProducerConfig { return c.Kafka },
			func(c config.Config) kafka.ConsumerConfig { return c.KafkaConsumer },

			postgres.New,
			goredis.New,
			kafka.NewProducer,

			// Session domain
			session.NewRepository,
			session.NewService,
			session.NewHandler,
			session.NewHeartbeatConsumer,
		),
		fx.Invoke(
			tracing.Register,
			func(cfg config.Config, log *slog.Logger) error {
				return postgres.RunMigrations(cfg.Postgres.DSN, "./migrations", log)
			},
			session.RegisterConsumer,
			server.Register,
		),
		fx.StopTimeout(cfg.ShutdownTimeout),
	).Run()
}
