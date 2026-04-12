package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type ConsumerConfig struct {
	Brokers  []string      `mapstructure:"brokers"`
	GroupID  string        `mapstructure:"group_id"`
	Topic    string        `mapstructure:"topic"`
	MinBytes int           `mapstructure:"min_bytes"`
	MaxBytes int           `mapstructure:"max_bytes"`
	MaxWait  time.Duration `mapstructure:"max_wait"`
}

// HandlerFunc processes a single Kafka message.
type HandlerFunc func(ctx context.Context, msg kafka.Message) error

type Consumer struct {
	reader *kafka.Reader
	logger *slog.Logger
}

func NewConsumer(cfg ConsumerConfig, logger *slog.Logger) *Consumer {
	readerCfg := kafka.ReaderConfig{
		Brokers:  cfg.Brokers,
		GroupID:  cfg.GroupID,
		Topic:    cfg.Topic,
		MinBytes: 1e3,  // 1 KB
		MaxBytes: 10e6, // 10 MB
	}
	if cfg.MinBytes > 0 {
		readerCfg.MinBytes = cfg.MinBytes
	}
	if cfg.MaxBytes > 0 {
		readerCfg.MaxBytes = cfg.MaxBytes
	}
	if cfg.MaxWait > 0 {
		readerCfg.MaxWait = cfg.MaxWait
	}

	return &Consumer{
		reader: kafka.NewReader(readerCfg),
		logger: logger,
	}
}

// Run starts consuming messages in a blocking loop until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, handler HandlerFunc) error {
	c.logger.Info("kafka consumer started", "topic", c.reader.Config().Topic, "group", c.reader.Config().GroupID)

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				c.logger.Info("kafka consumer stopped")
				return nil
			}
			c.logger.Error("kafka fetch error", "error", err)
			continue
		}

		msgCtx := extractTraceContext(ctx, msg.Headers)

		if err = handler(msgCtx, msg); err != nil {
			c.logger.Error("message processing failed",
				"topic", msg.Topic, "partition", msg.Partition,
				"offset", msg.Offset, "error", err,
			)
		}

		if err = c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("kafka commit error", "error", err)
		}
	}
}

// Close gracefully shuts down the consumer.
func (c *Consumer) Close() error {
	return c.reader.Close()
}

// HealthCheck verifies the consumer's connection by checking reader stats.
func (c *Consumer) HealthCheck() error {
	stats := c.reader.Stats()
	if stats.Errors > 0 {
		return fmt.Errorf("kafka consumer has %d errors", stats.Errors)
	}
	return nil
}

func extractTraceContext(ctx context.Context, headers []kafka.Header) context.Context {
	carrier := propagation.MapCarrier{}
	for _, h := range headers {
		carrier.Set(h.Key, string(h.Value))
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}
