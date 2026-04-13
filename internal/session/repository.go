package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"service/internal/apperror"
)

const (
	cacheTTL      = 24 * time.Hour
	cacheKeyPrefix = "session:"
)

// Repository handles persistence of PlaybackSession in PostgreSQL (durable store)
// and Redis (low-latency cache for cross-device sync).
type Repository struct {
	db     *pgxpool.Pool
	cache  *goredis.Client
	logger *slog.Logger
}

func NewRepository(db *pgxpool.Pool, cache *goredis.Client, logger *slog.Logger) *Repository {
	return &Repository{db: db, cache: cache, logger: logger}
}

// Get returns the session from Redis if available, falling back to PostgreSQL.
// Degraded mode (Redis down) transparently serves from DB.
func (r *Repository) Get(ctx context.Context, userID string) (*PlaybackSession, error) {
	key := cacheKeyPrefix + userID

	data, err := r.cache.Get(ctx, key).Bytes()
	switch {
	case err == nil:
		var s PlaybackSession
		if jsonErr := json.Unmarshal(data, &s); jsonErr == nil {
			return &s, nil
		}
		r.logger.Warn("corrupted cache entry, falling back to db", "user_id", userID)
	case errors.Is(err, goredis.Nil):
		// cache miss – normal path
	default:
		r.logger.Warn("redis unavailable, falling back to db", "user_id", userID, "error", err)
	}

	s, dbErr := r.getFromDB(ctx, userID)
	if dbErr != nil {
		return nil, dbErr
	}

	// warm the cache asynchronously so the caller is not blocked
	go func() {
		if cacheErr := r.SetCache(context.Background(), s); cacheErr != nil {
			r.logger.Warn("cache warm failed", "user_id", userID, "error", cacheErr)
		}
	}()

	return s, nil
}

func (r *Repository) getFromDB(ctx context.Context, userID string) (*PlaybackSession, error) {
	const q = `SELECT user_id, track_id, position, status, device_id, updated_at
	           FROM playback_sessions WHERE user_id = $1`

	var s PlaybackSession
	err := r.db.QueryRow(ctx, q, userID).Scan(
		&s.UserID, &s.TrackID, &s.Position, &s.Status, &s.DeviceID, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.NewNotFound("playback session not found")
	}
	if err != nil {
		return nil, apperror.NewInternal("db query failed", err)
	}
	return &s, nil
}

// Upsert inserts or fully replaces a session record in PostgreSQL.
func (r *Repository) Upsert(ctx context.Context, s *PlaybackSession) error {
	const q = `
		INSERT INTO playback_sessions (user_id, track_id, position, status, device_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE
		SET track_id   = EXCLUDED.track_id,
		    position   = EXCLUDED.position,
		    status     = EXCLUDED.status,
		    device_id  = EXCLUDED.device_id,
		    updated_at = EXCLUDED.updated_at`

	if _, err := r.db.Exec(ctx, q,
		s.UserID, s.TrackID, s.Position, s.Status, s.DeviceID, s.UpdatedAt,
	); err != nil {
		return fmt.Errorf("upsert playback session: %w", err)
	}
	return nil
}

// SetCache writes the session to Redis with a 24-hour TTL.
func (r *Repository) SetCache(ctx context.Context, s *PlaybackSession) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal session for cache: %w", err)
	}
	return r.cache.Set(ctx, cacheKeyPrefix+s.UserID, data, cacheTTL).Err()
}
