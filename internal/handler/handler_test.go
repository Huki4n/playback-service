package handler_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"

	"service/internal/handler"
)

func TestHealthz(t *testing.T) {
	h := handler.New(slog.Default())
	ctx := &fasthttp.RequestCtx{}
	h.Healthz(ctx)

	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode())

	var body map[string]string
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), &body))
	assert.Equal(t, "ok", body["status"])
}

func TestReadyz(t *testing.T) {
	h := handler.New(slog.Default())

	tests := []struct {
		name       string
		ready      bool
		wantStatus int
		wantBody   string
	}{
		{"ready", true, http.StatusOK, "ready"},
		{"not ready", false, http.StatusServiceUnavailable, "not ready"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.SetReady(tt.ready)
			ctx := &fasthttp.RequestCtx{}
			h.Readyz(ctx)

			assert.Equal(t, tt.wantStatus, ctx.Response.StatusCode())

			var body map[string]string
			require.NoError(t, json.Unmarshal(ctx.Response.Body(), &body))
			assert.Equal(t, tt.wantBody, body["status"])
		})
	}
}

func TestExample(t *testing.T) {
	h := handler.New(slog.Default())
	ctx := &fasthttp.RequestCtx{}
	h.Example(ctx)

	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode())

	var body map[string]string
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), &body))
	assert.Equal(t, "hello from service template", body["message"])
}

func TestWriteJSON_ContentType(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	handler.WriteJSON(ctx, http.StatusCreated, map[string]int{"count": 42})

	assert.Equal(t, http.StatusCreated, ctx.Response.StatusCode())
	assert.Contains(t, string(ctx.Response.Header.ContentType()), "application/json")
}
