package handler

import (
	"encoding/json"
	"log/slog"
	"service/internal/apperror"
	"sync/atomic"

	"github.com/valyala/fasthttp"
)

type Handler struct {
	logger *slog.Logger
	ready  atomic.Bool
}

func New(logger *slog.Logger) *Handler {
	h := &Handler{logger: logger}
	h.ready.Store(true)
	return h
}

// Healthz GoDoc
// @Summary     Проверка жизнеспособности сервиса
// @Description Возвращает статус "ok" если сервис запущен.
// @Tags        infrastructure
// @Produce     json
// @Success     200 {object} map[string]string
// @Router      /healthz [get]
func (h *Handler) Healthz(ctx *fasthttp.RequestCtx) {
	WriteJSON(ctx, fasthttp.StatusOK, map[string]string{"status": "ok"})
}

// Readyz GoDoc
// @Summary     Проверка готовности сервиса
// @Description Возвращает "ready" если сервис готов принимать трафик, "not ready" при shutdown.
// @Tags        infrastructure
// @Produce     json
// @Success     200 {object} map[string]string
// @Failure     503 {object} map[string]string
// @Router      /readyz [get]
func (h *Handler) Readyz(ctx *fasthttp.RequestCtx) {
	if !h.ready.Load() {
		WriteJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]string{"status": "not ready"})
		return
	}
	WriteJSON(ctx, fasthttp.StatusOK, map[string]string{"status": "ready"})
}

// SetReady toggles the readiness state (set to false during shutdown).
func (h *Handler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Example GoDoc
// @Summary     Пример endpoint
// @Description Возвращает приветственное сообщение. Используйте как основу для новых endpoint-ов.
// @Tags        example
// @Produce     json
// @Success     200 {object} map[string]string
// @Router      /example [get]
func (h *Handler) Example(ctx *fasthttp.RequestCtx) {
	WriteJSON(ctx, fasthttp.StatusOK, map[string]string{"message": "hello from service template"})
}

func WriteJSON(ctx *fasthttp.RequestCtx, status int, v any) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(status)
	if err := json.NewEncoder(ctx).Encode(v); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBodyString(`{"error":{"code":"INTERNAL_ERROR","message":"failed to encode response"}}`)
	}
}

func WriteError(ctx *fasthttp.RequestCtx, appErr *apperror.Error) {
	WriteJSON(ctx, appErr.HTTPStatus, apperror.ToResponse(appErr))
}
