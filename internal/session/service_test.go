package session

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"service/internal/apperror"
)

// ── mocks ────────────────────────────────────────────────────────────────────

type mockRepo struct {
	getResult   *PlaybackSession
	getErr      error
	upsertErr   error
	setCacheErr error
	upserted    *PlaybackSession
}

func (m *mockRepo) Get(_ context.Context, _ string) (*PlaybackSession, error) {
	return m.getResult, m.getErr
}

func (m *mockRepo) Upsert(_ context.Context, s *PlaybackSession) error {
	m.upserted = s
	return m.upsertErr
}

func (m *mockRepo) SetCache(_ context.Context, _ *PlaybackSession) error {
	return m.setCacheErr
}

type mockProducer struct {
	err    error
	called bool
}

func (m *mockProducer) Publish(_ context.Context, _ string, _, _ []byte) error {
	m.called = true
	return m.err
}

func newTestService(repo repoStore, prod eventPublisher) *Service {
	return &Service{repo: repo, producer: prod, logger: slog.Default()}
}

// ── Start ────────────────────────────────────────────────────────────────────

func TestService_Start_Success(t *testing.T) {
	repo := &mockRepo{}
	svc := newTestService(repo, &mockProducer{})

	sess, err := svc.Start(context.Background(), "user-1", StartRequest{
		TrackID:  "track-1",
		DeviceID: "phone",
		Position: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, "user-1", sess.UserID)
	assert.Equal(t, "track-1", sess.TrackID)
	assert.Equal(t, 10, sess.Position)
	assert.Equal(t, StatusPlaying, sess.Status)
	assert.Equal(t, "phone", sess.DeviceID)
	assert.NotZero(t, sess.UpdatedAt)
	// repo.Upsert must have been called
	require.NotNil(t, repo.upserted)
	assert.Equal(t, "user-1", repo.upserted.UserID)
}

func TestService_Start_UpsertFails(t *testing.T) {
	repo := &mockRepo{upsertErr: errors.New("db down")}
	svc := newTestService(repo, &mockProducer{})

	_, err := svc.Start(context.Background(), "user-1", StartRequest{
		TrackID: "track-1", DeviceID: "phone",
	})

	require.Error(t, err)
	var appErr *apperror.Error
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "INTERNAL_ERROR", appErr.Code)
}

// ── GetCurrent ───────────────────────────────────────────────────────────────

func TestService_GetCurrent_Delegates(t *testing.T) {
	want := &PlaybackSession{UserID: "u", TrackID: "t", Status: StatusPaused}
	repo := &mockRepo{getResult: want}
	svc := newTestService(repo, &mockProducer{})

	got, err := svc.GetCurrent(context.Background(), "u")

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestService_GetCurrent_NotFound(t *testing.T) {
	repo := &mockRepo{getErr: apperror.NewNotFound("not found")}
	svc := newTestService(repo, &mockProducer{})

	_, err := svc.GetCurrent(context.Background(), "u")

	require.Error(t, err)
	var appErr *apperror.Error
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "NOT_FOUND", appErr.Code)
}

// ── Heartbeat ────────────────────────────────────────────────────────────────

func TestService_Heartbeat_KafkaSuccess(t *testing.T) {
	repo := &mockRepo{}
	prod := &mockProducer{}
	svc := newTestService(repo, prod)

	err := svc.Heartbeat(context.Background(), "user-1", HeartbeatRequest{
		TrackID: "track-1", DeviceID: "phone", Position: 42,
	})

	require.NoError(t, err)
	assert.True(t, prod.called, "producer.Publish must be called")
	assert.Nil(t, repo.upserted, "DB fallback must NOT be called when Kafka succeeds")
}

func TestService_Heartbeat_KafkaFail_DBFallback(t *testing.T) {
	repo := &mockRepo{}
	prod := &mockProducer{err: errors.New("kafka down")}
	svc := newTestService(repo, prod)

	err := svc.Heartbeat(context.Background(), "user-1", HeartbeatRequest{
		TrackID: "track-1", DeviceID: "phone", Position: 42,
	})

	require.NoError(t, err)
	require.NotNil(t, repo.upserted, "DB fallback must be called when Kafka fails")
	assert.Equal(t, 42, repo.upserted.Position)
}

func TestService_Heartbeat_KafkaFail_DBFail(t *testing.T) {
	repo := &mockRepo{upsertErr: errors.New("db also down")}
	prod := &mockProducer{err: errors.New("kafka down")}
	svc := newTestService(repo, prod)

	err := svc.Heartbeat(context.Background(), "user-1", HeartbeatRequest{
		TrackID: "track-1", DeviceID: "phone", Position: 0,
	})

	require.Error(t, err)
	var appErr *apperror.Error
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "INTERNAL_ERROR", appErr.Code)
}

// ── Pause ────────────────────────────────────────────────────────────────────

func TestService_Pause_Success(t *testing.T) {
	existing := &PlaybackSession{UserID: "u", TrackID: "t", Position: 30, Status: StatusPlaying, DeviceID: "phone"}
	repo := &mockRepo{getResult: existing}
	svc := newTestService(repo, &mockProducer{})

	sess, err := svc.Pause(context.Background(), "u", "phone")

	require.NoError(t, err)
	assert.Equal(t, StatusPaused, sess.Status)
	assert.Equal(t, 30, sess.Position)
	require.NotNil(t, repo.upserted)
	assert.Equal(t, StatusPaused, repo.upserted.Status)
}

func TestService_Pause_NotFound(t *testing.T) {
	repo := &mockRepo{getErr: apperror.NewNotFound("not found")}
	svc := newTestService(repo, &mockProducer{})

	_, err := svc.Pause(context.Background(), "u", "phone")

	require.Error(t, err)
	var appErr *apperror.Error
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "NOT_FOUND", appErr.Code)
}

func TestService_Pause_UpsertFail(t *testing.T) {
	existing := &PlaybackSession{UserID: "u", TrackID: "t", Status: StatusPlaying}
	repo := &mockRepo{getResult: existing, upsertErr: errors.New("db down")}
	svc := newTestService(repo, &mockProducer{})

	_, err := svc.Pause(context.Background(), "u", "phone")

	require.Error(t, err)
	var appErr *apperror.Error
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "INTERNAL_ERROR", appErr.Code)
}

// ── Resume ───────────────────────────────────────────────────────────────────

func TestService_Resume_Success(t *testing.T) {
	existing := &PlaybackSession{UserID: "u", TrackID: "t", Position: 55, Status: StatusPaused, DeviceID: "phone"}
	repo := &mockRepo{getResult: existing}
	svc := newTestService(repo, &mockProducer{})

	sess, err := svc.Resume(context.Background(), "u", "laptop")

	require.NoError(t, err)
	assert.Equal(t, StatusPlaying, sess.Status)
	assert.Equal(t, "laptop", sess.DeviceID, "device must be switched")
	assert.Equal(t, 55, sess.Position, "position must be preserved")
}

func TestService_Resume_NotFound(t *testing.T) {
	repo := &mockRepo{getErr: apperror.NewNotFound("not found")}
	svc := newTestService(repo, &mockProducer{})

	_, err := svc.Resume(context.Background(), "u", "laptop")

	require.Error(t, err)
	var appErr *apperror.Error
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "NOT_FOUND", appErr.Code)
}

func TestService_Resume_UpsertFail(t *testing.T) {
	existing := &PlaybackSession{UserID: "u", TrackID: "t", Status: StatusPaused}
	repo := &mockRepo{getResult: existing, upsertErr: errors.New("db down")}
	svc := newTestService(repo, &mockProducer{})

	_, err := svc.Resume(context.Background(), "u", "laptop")

	require.Error(t, err)
	var appErr *apperror.Error
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "INTERNAL_ERROR", appErr.Code)
}
