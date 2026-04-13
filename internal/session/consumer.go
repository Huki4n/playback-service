package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/segmentio/kafka-go"
	"go.uber.org/fx"

	kafkapkg "service/internal/kafka"
)

// upsertStore is the minimal interface HeartbeatConsumer needs from the repository.
type upsertStore interface {
	Upsert(ctx context.Context, s *PlaybackSession) error
}

// HeartbeatConsumer reads playback.heartbeat events from Kafka and persists
// each session state update to PostgreSQL, decoupling the hot writing path.
type HeartbeatConsumer struct {
	consumer *kafkapkg.Consumer
	repo     upsertStore
	logger   *slog.Logger
}

func NewHeartbeatConsumer(cfg kafkapkg.ConsumerConfig, repo *Repository, logger *slog.Logger) *HeartbeatConsumer {
	return &HeartbeatConsumer{
		consumer: kafkapkg.NewConsumer(cfg, logger),
		repo:     repo,
		logger:   logger,
	}
}

// RegisterConsumer binds the consumer start/stop to the fx lifecycle.
func RegisterConsumer(lc fx.Lifecycle, hc *HeartbeatConsumer) {
	ctx, cancel := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go func() {
				if err := hc.consumer.Run(ctx, hc.handle); err != nil {
					hc.logger.Error("heartbeat consumer stopped with error", "error", err)
				}
			}()
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return hc.consumer.Close()
		},
	})
}

func (hc *HeartbeatConsumer) handle(ctx context.Context, msg kafka.Message) error {
	env, err := kafkapkg.ParseEnvelope(msg.Value)
	if err != nil {
		// Log and skip — malformed messages must not block the partition.
		hc.logger.Error("failed to parse heartbeat envelope", "offset", msg.Offset, "error", err)
		return nil
	}

	var sess PlaybackSession
	if err = json.Unmarshal(env.Payload, &sess); err != nil {
		hc.logger.Error("failed to unmarshal heartbeat payload", "offset", msg.Offset, "error", err)
		return nil
	}

	if err = hc.repo.Upsert(ctx, &sess); err != nil {
		return fmt.Errorf("persist heartbeat for user %s: %w", sess.UserID, err)
	}

	hc.logger.Debug("heartbeat persisted", "user_id", sess.UserID, "position", sess.Position)
	return nil
}
