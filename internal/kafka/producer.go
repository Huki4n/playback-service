package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/fx"
)

type ProducerConfig struct {
	Brokers      []string      `mapstructure:"brokers"`
	BatchTimeout time.Duration `mapstructure:"batch_timeout"`
	BatchSize    int           `mapstructure:"batch_size"`
	Async        bool          `mapstructure:"async"`
}

type Producer struct {
	writer *kafka.Writer
	logger *slog.Logger
}

func NewProducer(lc fx.Lifecycle, cfg ProducerConfig, logger *slog.Logger) *Producer {
	w := &kafka.Writer{
		Addr:                   kafka.TCP(cfg.Brokers...),
		Balancer:               &kafka.LeastBytes{},
		RequiredAcks:           kafka.RequireAll,
		MaxAttempts:            3,
		AllowAutoTopicCreation: true,
	}

	if cfg.BatchTimeout > 0 {
		w.BatchTimeout = cfg.BatchTimeout
	}
	if cfg.BatchSize > 0 {
		w.BatchSize = cfg.BatchSize
	}
	if cfg.Async {
		w.Async = true
	}

	p := &Producer{writer: w, logger: logger}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			logger.Info("closing kafka producer")
			return w.Close()
		},
	})

	return p
}

// Publish sends a message to the given topic, injecting OTel trace context into headers.
func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error {
	headers := injectTraceHeaders(ctx)

	msg := kafka.Message{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: headers,
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka publish to %s: %w", topic, err)
	}
	return nil
}

func injectTraceHeaders(ctx context.Context) []kafka.Header {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	headers := make([]kafka.Header, 0, len(carrier))
	for k, v := range carrier {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}
	return headers
}
