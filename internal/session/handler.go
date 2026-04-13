package session

import (
	"context"
	"errors"
	"log/slog"

	"github.com/valyala/fasthttp"

	"service/internal/apperror"
	"service/internal/handler"
	"service/internal/validator"
)

// sessionService defines the business operations required by Handler.
type sessionService interface {
	Start(ctx context.Context, userID string, req StartRequest) (*PlaybackSession, error)
	GetCurrent(ctx context.Context, userID string) (*PlaybackSession, error)
	Heartbeat(ctx context.Context, userID string, req HeartbeatRequest) error
	Pause(ctx context.Context, userID, deviceID string) (*PlaybackSession, error)
	Resume(ctx context.Context, userID, deviceID string) (*PlaybackSession, error)
}

// Handler exposes the session HTTP endpoints.
type Handler struct {
	svc    sessionService
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// userID extracts the X-User-ID header and writes an error response if missing.
func (h *Handler) userID(ctx *fasthttp.RequestCtx) (string, bool) {
	uid := string(ctx.Request.Header.Peek("X-User-ID"))
	if uid == "" {
		handler.WriteError(ctx, apperror.NewUnauthorized("X-User-ID header is required"))
		return "", false
	}
	return uid, true
}

// Start godoc
// @Summary     Start a playback session
// @Description Creates or replaces the active session for the authenticated user.
// @Tags        session
// @Accept      json
// @Produce     json
// @Param       X-User-ID header string             true "User identifier"
// @Param       body      body   StartRequest        true "Session start payload"
// @Success     201       {object} PlaybackSession
// @Failure     400       {object} apperror.Response
// @Failure     401       {object} apperror.Response
// @Failure     500       {object} apperror.Response
// @Router      /sessions [post]
func (h *Handler) Start(ctx *fasthttp.RequestCtx) {
	userID, ok := h.userID(ctx)
	if !ok {
		return
	}

	var req StartRequest
	if appErr := validator.BindJSON(ctx, &req); appErr != nil {
		handler.WriteError(ctx, appErr)
		return
	}

	sess, err := h.svc.Start(ctx, userID, req)
	if err != nil {
		h.writeServiceError(ctx, err)
		return
	}

	handler.WriteJSON(ctx, fasthttp.StatusCreated, sess)
}

// GetCurrent godoc
// @Summary     Get current playback session
// @Description Returns the latest session state; used by a new device to resume playback.
// @Tags        session
// @Produce     json
// @Param       X-User-ID header string true "User identifier"
// @Success     200 {object} PlaybackSession
// @Failure     401 {object} apperror.Response
// @Failure     404 {object} apperror.Response
// @Failure     500 {object} apperror.Response
// @Router      /sessions/current [get]
func (h *Handler) GetCurrent(ctx *fasthttp.RequestCtx) {
	userID, ok := h.userID(ctx)
	if !ok {
		return
	}

	sess, err := h.svc.GetCurrent(ctx, userID)
	if err != nil {
		h.writeServiceError(ctx, err)
		return
	}

	handler.WriteJSON(ctx, fasthttp.StatusOK, sess)
}

// Heartbeat godoc
// @Summary     Send a playback heartbeat
// @Description Updates the current track and position. Must be called every ≤5 seconds.
// @Tags        session
// @Accept      json
// @Produce     json
// @Param       X-User-ID header string           true "User identifier"
// @Param       body      body   HeartbeatRequest  true "Heartbeat payload"
// @Success     204
// @Failure     400 {object} apperror.Response
// @Failure     401 {object} apperror.Response
// @Failure     500 {object} apperror.Response
// @Router      /sessions/heartbeat [put]
func (h *Handler) Heartbeat(ctx *fasthttp.RequestCtx) {
	userID, ok := h.userID(ctx)
	if !ok {
		return
	}

	var req HeartbeatRequest
	if appErr := validator.BindJSON(ctx, &req); appErr != nil {
		handler.WriteError(ctx, appErr)
		return
	}

	if err := h.svc.Heartbeat(ctx, userID, req); err != nil {
		h.writeServiceError(ctx, err)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusNoContent)
}

// Pause godoc
// @Summary     Pause playback
// @Description Sets the session status to paused.
// @Tags        session
// @Accept      json
// @Produce     json
// @Param       X-User-ID header string             true "User identifier"
// @Param       body      body   PauseResumeRequest  true "Device identifier"
// @Success     200 {object} PlaybackSession
// @Failure     400 {object} apperror.Response
// @Failure     401 {object} apperror.Response
// @Failure     404 {object} apperror.Response
// @Failure     500 {object} apperror.Response
// @Router      /sessions/pause [put]
func (h *Handler) Pause(ctx *fasthttp.RequestCtx) {
	userID, ok := h.userID(ctx)
	if !ok {
		return
	}

	var req PauseResumeRequest
	if appErr := validator.BindJSON(ctx, &req); appErr != nil {
		handler.WriteError(ctx, appErr)
		return
	}

	sess, err := h.svc.Pause(ctx, userID, req.DeviceID)
	if err != nil {
		h.writeServiceError(ctx, err)
		return
	}

	handler.WriteJSON(ctx, fasthttp.StatusOK, sess)
}

// Resume godoc
// @Summary     Resume playback
// @Description Sets the session status to playing; supports cross-device resume.
// @Tags        session
// @Accept      json
// @Produce     json
// @Param       X-User-ID header string             true "User identifier"
// @Param       body      body   PauseResumeRequest  true "Device identifier"
// @Success     200 {object} PlaybackSession
// @Failure     400 {object} apperror.Response
// @Failure     401 {object} apperror.Response
// @Failure     404 {object} apperror.Response
// @Failure     500 {object} apperror.Response
// @Router      /sessions/resume [put]
func (h *Handler) Resume(ctx *fasthttp.RequestCtx) {
	userID, ok := h.userID(ctx)
	if !ok {
		return
	}

	var req PauseResumeRequest
	if appErr := validator.BindJSON(ctx, &req); appErr != nil {
		handler.WriteError(ctx, appErr)
		return
	}

	sess, err := h.svc.Resume(ctx, userID, req.DeviceID)
	if err != nil {
		h.writeServiceError(ctx, err)
		return
	}

	handler.WriteJSON(ctx, fasthttp.StatusOK, sess)
}

func (h *Handler) writeServiceError(ctx *fasthttp.RequestCtx, err error) {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		handler.WriteError(ctx, appErr)
		return
	}
	h.logger.Error("unexpected service error", "error", err)
	handler.WriteError(ctx, apperror.NewInternal("unexpected error", err))
}
