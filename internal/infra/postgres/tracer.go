package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type (
	traceKey   struct{}
	traceState struct{ collection string }
	tracerKey  struct {
		collection string
		failed     bool
	}
)
type queryTracer struct{ tracers map[tracerKey]*otelpgx.Tracer }

func newQueryTracer(traces trace.TracerProvider, metrics metric.MeterProvider) *queryTracer {
	result := &queryTracer{tracers: make(map[tracerKey]*otelpgx.Tracer)}
	for _, collection := range []string{"", "trips", "trip_status_history", "trip_idempotency_keys"} {
		for _, failed := range []bool{false, true} {
			key := tracerKey{collection: collection, failed: failed}
			errorType := ""
			if key.failed {
				errorType = "database_error"
			}
			result.tracers[key] = otelpgx.NewTracer(otelpgx.WithTracerProvider(traces), otelpgx.WithMeterProvider(metrics), otelpgx.WithDisableSQLStatementInAttributes(), otelpgx.WithMetricOperationName(otelpgx.SQLOperationName), otelpgx.WithTracerAttributes(attribute.String("db.collection.name", key.collection)), otelpgx.WithMeterAttributes(attribute.String("db.collection.name", key.collection), attribute.String("error.type", errorType)))
		}
	}
	return result
}

func (tracer *queryTracer) TraceQueryStart(ctx context.Context, connection *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	collection := ""
	for _, table := range []string{"trip_idempotency_keys", "trip_status_history", "trips"} {
		if strings.Contains(data.SQL, table) {
			collection = table
			break
		}
	}
	ctx = tracer.tracers[tracerKey{collection: collection}].TraceQueryStart(ctx, connection, data)
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.operation.name", otelpgx.SQLOperationName(ctx, data.SQL)))
	return context.WithValue(ctx, traceKey{}, traceState{collection: collection})
}

func (tracer *queryTracer) TraceQueryEnd(ctx context.Context, connection *pgx.Conn, data pgx.TraceQueryEndData) {
	state, _ := ctx.Value(traceKey{}).(traceState)
	failed := data.Err != nil && !errors.Is(data.Err, pgx.ErrNoRows)
	if failed {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("error.type", "database_error"))
	}
	tracer.tracers[tracerKey{collection: state.collection, failed: failed}].TraceQueryEnd(ctx, connection, data)
}
