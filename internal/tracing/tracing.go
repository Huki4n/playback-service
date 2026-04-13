package tracing

import (
	"context"
	"fmt"
	"log/slog"
	"service/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Register initialises OpenTelemetry tracing and binds shutdown to the fx lifecycle.
// If tracing is disabled via config, this is a no-op.
func Register(lc fx.Lifecycle, cfg config.Config, logger *slog.Logger) error {
	if !cfg.TracingEnabled {
		logger.Info("tracing disabled")
		return nil
	}

	shutdown, err := setup(cfg.ServiceName, cfg.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}

	logger.Info("tracing enabled", "otlp_endpoint", cfg.OTLPEndpoint)

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("shutting down tracer provider")
			return shutdown(ctx)
		},
	})

	return nil
}

func setup(serviceName, otlpEndpoint string) (func(context.Context) error, error) {
	ctx := context.Background()

	// NewSchemaless avoids a schema URL conflict between resource.Default()
	// (which uses semconv v1.40.0) and our semconv/v1.26.0 import.
	// resource.Merge accepts a schemaless resource and adopts the schema URL
	// from the non-empty side (resource.Default()).
	custom := resource.NewSchemaless(semconv.ServiceName(serviceName))
	res, err := resource.Merge(resource.Default(), custom)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	conn, err := grpc.NewClient(otlpEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("create exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
