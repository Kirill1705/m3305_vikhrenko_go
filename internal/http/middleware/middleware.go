package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/requestinfo"
	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/response"
)

const (
	methodAttribute = "http.request.method"
	routeAttribute  = "http.route"
	statusAttribute = "http.response.status_code"
)

type writer struct {
	http.ResponseWriter
	info  *requestinfo.Info
	wrote bool
}

func (w *writer) WriteHeader(status int) {
	if !w.wrote {
		w.info.Status = status
		w.wrote = true
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *writer) Write(data []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}
func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func after(r *http.Request, finish func()) {
	if value := recover(); value != nil {
		info := requestinfo.From(r.Context())
		if len(info.Stack) == 0 {
			info.Err = fmt.Errorf("panic: %v", value)
			info.Stack = debug.Stack()
			info.ErrorType = "panic"
		}
		info.Status = http.StatusInternalServerError
		finish()
		panic(value)
	}
	finish()
}

func Recover(logger *slog.Logger, route func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := &requestinfo.Info{Route: route(r), Status: http.StatusOK}
			r = r.WithContext(requestinfo.With(r.Context(), info))
			wrapped := &writer{ResponseWriter: w, info: info}
			defer func() {
				if value := recover(); value != nil {
					if !info.Logged {
						logger.ErrorContext(r.Context(), "request panicked", "panic", value, "stack", string(debug.Stack()))
					}
					response.Error(wrapped, r, fmt.Errorf("request panicked: %v", value))
				}
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}

func Tracing(provider trace.TracerProvider) func(http.Handler) http.Handler {
	tracer := provider.Tracer("tripgo/http")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := requestinfo.From(r.Context())
			ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tracer.Start(ctx, r.Method+" "+info.Route, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String(methodAttribute, r.Method), attribute.String(routeAttribute, info.Route)))
			r = r.WithContext(ctx)
			defer after(r, func() {
				span.SetAttributes(attribute.Int(statusAttribute, info.Status))
				if info.Err != nil {
					span.RecordError(info.Err)
					span.SetAttributes(attribute.String("error.type", info.ErrorType))
					span.SetStatus(codes.Error, info.ErrorType)
				}
				span.End()
			})
			next.ServeHTTP(w, r)
		})
	}
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			defer after(r, func() {
				info := requestinfo.From(r.Context())
				level := slog.LevelInfo
				if info.Status >= http.StatusInternalServerError {
					level = slog.LevelError
				}
				fields := []any{"method", r.Method, "route", info.Route, "status", info.Status, "duration_ms", float64(time.Since(started)) / float64(time.Millisecond)}
				if info.Err != nil {
					fields = append(fields, "error", info.Err)
				}
				if len(info.Stack) != 0 {
					fields = append(fields, "stack", string(info.Stack))
				}
				logger.Log(r.Context(), level, "request handled", fields...)
				info.Logged = true
			})
			next.ServeHTTP(w, r)
		})
	}
}

type Metrics struct {
	duration metric.Float64Histogram
	active   metric.Int64UpDownCounter
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .1, .25, .5, .8, 1, 2.5, 5, 10))
	if err != nil {
		return nil, fmt.Errorf("create HTTP duration: %w", err)
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, fmt.Errorf("create HTTP active requests: %w", err)
	}
	return &Metrics{duration: duration, active: active}, nil
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		info := requestinfo.From(r.Context())
		attrs := []attribute.KeyValue{attribute.String(methodAttribute, r.Method), attribute.String(routeAttribute, info.Route)}
		m.active.Add(r.Context(), 1, metric.WithAttributes(attrs...))
		defer after(r, func() {
			m.active.Add(r.Context(), -1, metric.WithAttributes(attrs...))
			attrs = append(attrs, attribute.Int(statusAttribute, info.Status))
			m.duration.Record(r.Context(), time.Since(started).Seconds(), metric.WithAttributes(attrs...))
		})
		next.ServeHTTP(w, r)
	})
}
