package main

import (
	"go.uber.org/fx"

	"service/internal/config"
	"service/internal/handler"
	"service/internal/logger"
	"service/internal/server"
	"service/internal/tracing"
)

// @title           Service API
// @version         1.0
// @description     API шаблона микросервиса.
// @host            localhost:8080
// @BasePath        /api/v1
func main() {
	cfg := config.Load()

	fx.New(
		fx.Supply(cfg),
		fx.Provide(
			logger.New,
			handler.New,
		),
		fx.Invoke(
			tracing.Register,
			server.Register,
		),
		fx.StopTimeout(cfg.ShutdownTimeout),
	).Run()
}
