package middleware_test

import (
	"log/slog"
	"net/http"
	"service/internal/middleware"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	called := false

	handler := middleware.RequestID(func(ctx *fasthttp.RequestCtx) {
		called = true
	})
	handler(ctx)

	assert.True(t, called)
	id := string(ctx.Response.Header.Peek(middleware.RequestIDHeader))
	assert.NotEmpty(t, id)
	assert.Len(t, id, 32) // 16 bytes hex-encoded
}

func TestRequestID_PreservesExisting(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set(middleware.RequestIDHeader, "my-custom-id")

	handler := middleware.RequestID(func(ctx *fasthttp.RequestCtx) {})
	handler(ctx)

	assert.Equal(t, "my-custom-id", string(ctx.Response.Header.Peek(middleware.RequestIDHeader)))
	assert.Equal(t, "my-custom-id", middleware.GetRequestID(ctx))
}

func TestRecoverer_CatchesPanic(t *testing.T) {
	logger := slog.Default()
	ctx := &fasthttp.RequestCtx{}

	handler := middleware.Recoverer(logger)(func(ctx *fasthttp.RequestCtx) {
		panic("test panic")
	})
	handler(ctx)

	assert.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode())
}

func TestRecoverer_PassesThrough(t *testing.T) {
	logger := slog.Default()
	ctx := &fasthttp.RequestCtx{}

	handler := middleware.Recoverer(logger)(func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(http.StatusOK)
	})
	handler(ctx)

	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode())
}

func TestMetrics_UsesRoutePattern(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue(middleware.RoutePatternKey, "/api/v1/test")

	called := false
	handler := middleware.Metrics(func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(http.StatusOK)
	})
	handler(ctx)

	assert.True(t, called)
}

func TestChain_Order(t *testing.T) {
	var order []string

	mw1 := func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			order = append(order, "mw1-before")
			next(ctx)
			order = append(order, "mw1-after")
		}
	}
	mw2 := func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			order = append(order, "mw2-before")
			next(ctx)
			order = append(order, "mw2-after")
		}
	}

	chain := middleware.Chain(mw1, mw2)
	handler := chain(func(ctx *fasthttp.RequestCtx) {
		order = append(order, "handler")
	})

	ctx := &fasthttp.RequestCtx{}
	handler(ctx)

	assert.Equal(t, []string{
		"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after",
	}, order)
}
