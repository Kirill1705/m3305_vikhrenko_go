package telemetry

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

type traceHandler struct{ slog.Handler }

func (handler traceHandler) Handle(ctx context.Context, record slog.Record) error {
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		record = record.Clone()
		record.AddAttrs(slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	}
	return handler.Handler.Handle(ctx, record)
}

func (handler traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{Handler: handler.Handler.WithAttrs(attrs)}
}

func (handler traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{Handler: handler.Handler.WithGroup(name)}
}

func NewLogger(level slog.Level, service string) *slog.Logger {
	return slog.New(traceHandler{Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})}).With("service", service)
}
