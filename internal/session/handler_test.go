package session

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"

	"service/internal/apperror"
)

// ── mock service ─────────────────────────────────────────────────────────────

type mockSvc struct {
	startResult      *PlaybackSession
	startErr         error
	getCurrentResult *PlaybackSession
	getCurrentErr    error
	heartbeatErr     error
	pauseResult      *PlaybackSession
	pauseErr         error
	resumeResult     *PlaybackSession
	resumeErr        error
}

func (m *mockSvc) Start(_ context.Context, _ string, _ StartRequest) (*PlaybackSession, error) {
	return m.startResult, m.startErr
}
func (m *mockSvc) GetCurrent(_ context.Context, _ string) (*PlaybackSession, error) {
	return m.getCurrentResult, m.getCurrentErr
}
func (m *mockSvc) Heartbeat(_ context.Context, _ string, _ HeartbeatRequest) error {
	return m.heartbeatErr
}
func (m *mockSvc) Pause(_ context.Context, _, _ string) (*PlaybackSession, error) {
	return m.pauseResult, m.pauseErr
}
func (m *mockSvc) Resume(_ context.Context, _, _ string) (*PlaybackSession, error) {
	return m.resumeResult, m.resumeErr
}

func newTestHandler(svc sessionService) *Handler {
	return &Handler{svc: svc, logger: slog.Default()}
}

// helpers

func makeCtx(method, userID string, body []byte) *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(method)
	if userID != "" {
		ctx.Request.Header.Set("X-User-ID", userID)
	}
	if body != nil {
		ctx.Request.SetBody(body)
		ctx.Request.Header.SetContentType("application/json")
	}
	return ctx
}

func decodeBody(t *testing.T, ctx *fasthttp.RequestCtx, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), dst))
}

func sampleSession() *PlaybackSession {
	return &PlaybackSession{
		UserID:    "user-1",
		TrackID:   "track-1",
		Position:  0,
		Status:    StatusPlaying,
		DeviceID:  "phone",
		UpdatedAt: time.Now().UTC(),
	}
}

// ── Start ────────────────────────────────────────────────────────────────────

func TestHandler_Start_MissingUserID(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("POST", "", []byte(`{"track_id":"t","device_id":"d"}`))
	h.Start(ctx)
	assert.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode())
}

func TestHandler_Start_EmptyBody(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("POST", "user-1", nil)
	h.Start(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode())
}

func TestHandler_Start_InvalidJSON(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("POST", "user-1", []byte(`not-json`))
	h.Start(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode())
}

func TestHandler_Start_MissingRequiredField(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	// missing device_id
	ctx := makeCtx("POST", "user-1", []byte(`{"track_id":"t"}`))
	h.Start(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode())
}

func TestHandler_Start_Success(t *testing.T) {
	sess := sampleSession()
	h := newTestHandler(&mockSvc{startResult: sess})
	ctx := makeCtx("POST", "user-1", []byte(`{"track_id":"track-1","device_id":"phone"}`))
	h.Start(ctx)

	assert.Equal(t, http.StatusCreated, ctx.Response.StatusCode())
	var got PlaybackSession
	decodeBody(t, ctx, &got)
	assert.Equal(t, sess.UserID, got.UserID)
	assert.Equal(t, StatusPlaying, got.Status)
}

func TestHandler_Start_ServiceError(t *testing.T) {
	h := newTestHandler(&mockSvc{startErr: apperror.NewInternal("oops", errors.New("db"))})
	ctx := makeCtx("POST", "user-1", []byte(`{"track_id":"t","device_id":"d"}`))
	h.Start(ctx)
	assert.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode())
}

// ── GetCurrent ───────────────────────────────────────────────────────────────

func TestHandler_GetCurrent_MissingUserID(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("GET", "", nil)
	h.GetCurrent(ctx)
	assert.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode())
}

func TestHandler_GetCurrent_Success(t *testing.T) {
	sess := sampleSession()
	h := newTestHandler(&mockSvc{getCurrentResult: sess})
	ctx := makeCtx("GET", "user-1", nil)
	h.GetCurrent(ctx)

	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode())
	var got PlaybackSession
	decodeBody(t, ctx, &got)
	assert.Equal(t, sess.TrackID, got.TrackID)
}

func TestHandler_GetCurrent_NotFound(t *testing.T) {
	h := newTestHandler(&mockSvc{getCurrentErr: apperror.NewNotFound("no session")})
	ctx := makeCtx("GET", "user-1", nil)
	h.GetCurrent(ctx)
	assert.Equal(t, http.StatusNotFound, ctx.Response.StatusCode())
}

// ── Heartbeat ────────────────────────────────────────────────────────────────

func TestHandler_Heartbeat_MissingUserID(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("PUT", "", []byte(`{"track_id":"t","device_id":"d","position_sec":0}`))
	h.Heartbeat(ctx)
	assert.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode())
}

func TestHandler_Heartbeat_Success(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("PUT", "user-1", []byte(`{"track_id":"t","device_id":"d","position_sec":42}`))
	h.Heartbeat(ctx)
	assert.Equal(t, http.StatusNoContent, ctx.Response.StatusCode())
}

func TestHandler_Heartbeat_InvalidBody(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	// missing required track_id
	ctx := makeCtx("PUT", "user-1", []byte(`{"device_id":"d"}`))
	h.Heartbeat(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode())
}

func TestHandler_Heartbeat_ServiceError(t *testing.T) {
	h := newTestHandler(&mockSvc{heartbeatErr: apperror.NewInternal("fail", errors.New("x"))})
	ctx := makeCtx("PUT", "user-1", []byte(`{"track_id":"t","device_id":"d","position_sec":0}`))
	h.Heartbeat(ctx)
	assert.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode())
}

// ── Pause ────────────────────────────────────────────────────────────────────

func TestHandler_Pause_MissingUserID(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("PUT", "", []byte(`{"device_id":"phone"}`))
	h.Pause(ctx)
	assert.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode())
}

func TestHandler_Pause_Success(t *testing.T) {
	sess := sampleSession()
	sess.Status = StatusPaused
	h := newTestHandler(&mockSvc{pauseResult: sess})
	ctx := makeCtx("PUT", "user-1", []byte(`{"device_id":"phone"}`))
	h.Pause(ctx)

	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode())
	var got PlaybackSession
	decodeBody(t, ctx, &got)
	assert.Equal(t, StatusPaused, got.Status)
}

func TestHandler_Pause_NotFound(t *testing.T) {
	h := newTestHandler(&mockSvc{pauseErr: apperror.NewNotFound("no session")})
	ctx := makeCtx("PUT", "user-1", []byte(`{"device_id":"phone"}`))
	h.Pause(ctx)
	assert.Equal(t, http.StatusNotFound, ctx.Response.StatusCode())
}

func TestHandler_Pause_MissingDeviceID(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("PUT", "user-1", []byte(`{}`))
	h.Pause(ctx)
	assert.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode())
}

// ── Resume ───────────────────────────────────────────────────────────────────

func TestHandler_Resume_MissingUserID(t *testing.T) {
	h := newTestHandler(&mockSvc{})
	ctx := makeCtx("PUT", "", []byte(`{"device_id":"laptop"}`))
	h.Resume(ctx)
	assert.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode())
}

func TestHandler_Resume_Success(t *testing.T) {
	sess := sampleSession()
	sess.DeviceID = "laptop"
	h := newTestHandler(&mockSvc{resumeResult: sess})
	ctx := makeCtx("PUT", "user-1", []byte(`{"device_id":"laptop"}`))
	h.Resume(ctx)

	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode())
	var got PlaybackSession
	decodeBody(t, ctx, &got)
	assert.Equal(t, StatusPlaying, got.Status)
	assert.Equal(t, "laptop", got.DeviceID)
}

func TestHandler_Resume_NotFound(t *testing.T) {
	h := newTestHandler(&mockSvc{resumeErr: apperror.NewNotFound("no session")})
	ctx := makeCtx("PUT", "user-1", []byte(`{"device_id":"laptop"}`))
	h.Resume(ctx)
	assert.Equal(t, http.StatusNotFound, ctx.Response.StatusCode())
}
