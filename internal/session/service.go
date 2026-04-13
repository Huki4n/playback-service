package session

import (
	"context"
	"log/slog"
	"service/internal/apperror"
	"service/internal/kafka"
	"time"
)

const heartbeatTopic = "playback.heartbeat"

// repoStore defines the data access methods required by Service.
type repoStore interface {
	Get(ctx context.Context, userID string) (*PlaybackSession, error)
	Upsert(ctx context.Context, s *PlaybackSession) error
	SetCache(ctx context.Context, s *PlaybackSession) error
}

// eventPublisher abstracts Kafka publishing for testability.
type eventPublisher interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

// Service contains the business logic for playback session management.
type Service struct {
	repo     repoStore
	producer eventPublisher
	logger   *slog.Logger
}

func NewService(repo *Repository, producer *kafka.Producer, logger *slog.Logger) *Service {
	return &Service{repo: repo, producer: producer, logger: logger}
}

// Start creates or replaces the active session for the user and marks it as playing.
func (s *Service) Start(ctx context.Context, userID string, req StartRequest) (*PlaybackSession, error) {
	sess := &PlaybackSession{
		UserID:    userID,
		TrackID:   req.TrackID,
		Position:  req.Position,
		Status:    StatusPlaying,
		DeviceID:  req.DeviceID,
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.repo.Upsert(ctx, sess); err != nil {
		return nil, apperror.NewInternal("failed to start session", err)
	}
	if err := s.repo.SetCache(ctx, sess); err != nil {
		s.logger.Warn("failed to cache new session", "user_id", userID, "error", err)
	}
	return sess, nil
}

// GetCurrent returns the most recent session state for the user.
func (s *Service) GetCurrent(ctx context.Context, userID string) (*PlaybackSession, error) {
	return s.repo.Get(ctx, userID)
}

// Heartbeat updates the playback position.
// Redis is updated immediately for low-latency cross-device sync.
// A Kafka event triggers the async DB write; if Kafka is unavailable the
// service falls back to a synchronous DB write so data is never lost.
func (s *Service) Heartbeat(ctx context.Context, userID string, req HeartbeatRequest) error {
	sess := &PlaybackSession{
		UserID:    userID,
		TrackID:   req.TrackID,
		Position:  req.Position,
		Status:    StatusPlaying,
		DeviceID:  req.DeviceID,
		UpdatedAt: time.Now().UTC(),
	}

	// Fast path: update the cache so other devices see ≤5 s stale data.
	if err := s.repo.SetCache(ctx, sess); err != nil {
		s.logger.Warn("redis cache miss on heartbeat", "user_id", userID, "error", err)
	}

	// Publish heartbeat event; the consumer will persist it to the DB.
	payload, err := kafka.NewEnvelope("playback.heartbeat", 1, sess)
	if err != nil {
		return apperror.NewInternal("failed to encode heartbeat event", err)
	}

	if pubErr := s.producer.Publish(ctx, heartbeatTopic, []byte(userID), payload); pubErr != nil {
		s.logger.Error("kafka publish failed, writing heartbeat directly to db",
			"user_id", userID, "error", pubErr)
		// Fallback: synchronous DB write so position is never lost.
		if dbErr := s.repo.Upsert(ctx, sess); dbErr != nil {
			return apperror.NewInternal("failed to persist heartbeat", dbErr)
		}
	}

	return nil
}

// Pause sets the session status to paused.
func (s *Service) Pause(ctx context.Context, userID, deviceID string) (*PlaybackSession, error) {
	sess, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	sess.Status = StatusPaused
	sess.DeviceID = deviceID
	sess.UpdatedAt = time.Now().UTC()

	if err = s.repo.Upsert(ctx, sess); err != nil {
		return nil, apperror.NewInternal("failed to pause session", err)
	}
	if err = s.repo.SetCache(ctx, sess); err != nil {
		s.logger.Warn("failed to cache paused session", "user_id", userID, "error", err)
	}
	return sess, nil
}

// Resume sets the session status back to playing, optionally from a different device.
func (s *Service) Resume(ctx context.Context, userID, deviceID string) (*PlaybackSession, error) {
	sess, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	sess.Status = StatusPlaying
	sess.DeviceID = deviceID
	sess.UpdatedAt = time.Now().UTC()

	if err = s.repo.Upsert(ctx, sess); err != nil {
		return nil, apperror.NewInternal("failed to resume session", err)
	}
	if err = s.repo.SetCache(ctx, sess); err != nil {
		s.logger.Warn("failed to cache resumed session", "user_id", userID, "error", err)
	}
	return sess, nil
}
