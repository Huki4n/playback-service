package session

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kafkapkg "service/internal/kafka"
)

// ── mock ─────────────────────────────────────────────────────────────────────

type mockUpsertStore struct {
	err          error
	lastUpserted *PlaybackSession
}

func (m *mockUpsertStore) Upsert(_ context.Context, s *PlaybackSession) error {
	m.lastUpserted = s
	return m.err
}

func newTestConsumer(store upsertStore) *HeartbeatConsumer {
	return &HeartbeatConsumer{
		consumer: nil, // not used by handle()
		repo:     store,
		logger:   slog.Default(),
	}
}

// buildMsg creates a Kafka message containing a valid heartbeat envelope.
func buildMsg(t *testing.T, sess *PlaybackSession) kafka.Message {
	t.Helper()
	data, err := kafkapkg.NewEnvelope("playback.heartbeat", 1, sess)
	require.NoError(t, err)
	return kafka.Message{Value: data}
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestConsumer_Handle_Success(t *testing.T) {
	store := &mockUpsertStore{}
	hc := newTestConsumer(store)

	sess := &PlaybackSession{
		UserID:    "user-1",
		TrackID:   "track-1",
		Position:  42,
		Status:    StatusPlaying,
		DeviceID:  "phone",
		UpdatedAt: time.Now().UTC(),
	}

	err := hc.handle(context.Background(), buildMsg(t, sess))

	require.NoError(t, err)
	require.NotNil(t, store.lastUpserted)
	assert.Equal(t, "user-1", store.lastUpserted.UserID)
	assert.Equal(t, 42, store.lastUpserted.Position)
}

func TestConsumer_Handle_MalformedEnvelope(t *testing.T) {
	store := &mockUpsertStore{}
	hc := newTestConsumer(store)

	// garbage bytes — ParseEnvelope must fail gracefully (no error returned)
	err := hc.handle(context.Background(), kafka.Message{Value: []byte(`not-json`)})

	require.NoError(t, err, "malformed envelope must be skipped, not propagated")
	assert.Nil(t, store.lastUpserted)
}

func TestConsumer_Handle_MalformedPayload(t *testing.T) {
	store := &mockUpsertStore{}
	hc := newTestConsumer(store)

	// Outer envelope is valid JSON, but payload is a JSON array —
	// incompatible with PlaybackSession struct, so json.Unmarshal fails.
	data, err := json.Marshal(map[string]interface{}{
		"type":      "playback.heartbeat",
		"version":   1,
		"timestamp": time.Now().UTC(),
		"payload":   []int{1, 2, 3},
	})
	require.NoError(t, err)

	handleErr := hc.handle(context.Background(), kafka.Message{Value: data})

	require.NoError(t, handleErr, "incompatible payload must be skipped gracefully")
	assert.Nil(t, store.lastUpserted)
}

func TestConsumer_Handle_UpsertError(t *testing.T) {
	store := &mockUpsertStore{err: errors.New("db down")}
	hc := newTestConsumer(store)

	sess := &PlaybackSession{
		UserID:    "user-1",
		TrackID:   "track-1",
		Position:  0,
		UpdatedAt: time.Now().UTC(),
	}

	err := hc.handle(context.Background(), buildMsg(t, sess))

	require.Error(t, err, "upsert failure must be propagated so Kafka retries")
	assert.Contains(t, err.Error(), "user-1")
}
