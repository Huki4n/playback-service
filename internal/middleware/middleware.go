package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type Middleware func(fasthttp.RequestHandler) fasthttp.RequestHandler

// Chain composes middlewares so the first in the list is the outermost.
func Chain(mws ...Middleware) Middleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// --------------- Request ID ---------------

const RequestIDHeader = "X-Request-Id"

func RequestID(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		id := string(ctx.Request.Header.Peek(RequestIDHeader))
		if id == "" {
			id = generateID()
		}
		ctx.Response.Header.Set(RequestIDHeader, id)
		ctx.SetUserValue("request_id", id)
		next(ctx)
	}
}

func GetRequestID(ctx *fasthttp.RequestCtx) string {
	if v, ok := ctx.UserValue("request_id").(string); ok {
		return v
	}
	return ""
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// --------------- OpenTelemetry Tracing ---------------

const otelCtxKey = "otel_ctx"

// Tracing extracts incoming W3C trace context, creates a server span
// for every request and stores the span context in UserValue for downstream use.
func Tracing(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	tracer := otel.Tracer("http-server")

	return func(ctx *fasthttp.RequestCtx) {
		carrier := &fasthttpCarrier{req: &ctx.Request}
		parentCtx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)

		spanCtx, span := tracer.Start(parentCtx, fmt.Sprintf("%s %s", ctx.Method(), ctx.Path()),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(string(ctx.Method())),
				semconv.URLPath(string(ctx.Path())),
				attribute.String("net.peer.ip", ctx.RemoteAddr().String()),
			),
		)
		defer span.End()

		ctx.SetUserValue(otelCtxKey, spanCtx)

		next(ctx)

		status := ctx.Response.StatusCode()
		span.SetAttributes(semconv.HTTPResponseStatusCode(status))
		if status >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		}
	}
}

// SpanContext retrieves the OTel context stored by the Tracing middleware.
func SpanContext(ctx *fasthttp.RequestCtx) context.Context {
	if v, ok := ctx.UserValue(otelCtxKey).(context.Context); ok {
		return v
	}
	return context.Background()
}

// fasthttpCarrier implements propagation.TextMapCarrier over fasthttp request headers.
type fasthttpCarrier struct {
	req *fasthttp.Request
}

func (c *fasthttpCarrier) Get(key string) string {
	return string(c.req.Header.Peek(key))
}

func (c *fasthttpCarrier) Set(key, value string) {
	c.req.Header.Set(key, value)
}

func (c *fasthttpCarrier) Keys() []string {
	var keys []string
	c.req.Header.VisitAll(func(k, _ []byte) {
		keys = append(keys, string(k))
	})
	return keys
}

// --------------- Logger ---------------

func Logger(logger *slog.Logger) Middleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			start := time.Now()

			next(ctx)

			attrs := []any{
				"method", string(ctx.Method()),
				"path", string(ctx.Path()),
				"status", ctx.Response.StatusCode(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", GetRequestID(ctx),
				"remote_addr", ctx.RemoteAddr().String(),
			}

			if sc := trace.SpanFromContext(SpanContext(ctx)).SpanContext(); sc.HasTraceID() {
				attrs = append(attrs, "trace_id", sc.TraceID().String())
			}

			logger.Info("request completed", attrs...)
		}
	}
}

// --------------- Recoverer ---------------

func Recoverer(logger *slog.Logger) Middleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"error", rec,
						"stack", string(debug.Stack()),
						"path", string(ctx.Path()),
						"request_id", GetRequestID(ctx),
					)

					if span := trace.SpanFromContext(SpanContext(ctx)); span.IsRecording() {
						span.SetStatus(codes.Error, fmt.Sprintf("panic: %v", rec))
					}

					ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
				}
			}()
			next(ctx)
		}
	}
}

// --------------- Prometheus Metrics ---------------

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests by method, path, and status.",
		},
		[]string{"method", "path", "status"},
	)
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Histogram of HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

const RoutePatternKey = "route_pattern"

func Metrics(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		start := time.Now()

		next(ctx)

		method := string(ctx.Method())
		pattern := routePattern(ctx)
		status := strconv.Itoa(ctx.Response.StatusCode())

		httpRequestsTotal.WithLabelValues(method, pattern, status).Inc()
		httpRequestDuration.WithLabelValues(method, pattern).Observe(time.Since(start).Seconds())
	}
}

func routePattern(ctx *fasthttp.RequestCtx) string {
	if v, ok := ctx.UserValue(RoutePatternKey).(string); ok && v != "" {
		return v
	}
	return "unknown"
}
