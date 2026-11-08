package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Kirill1705/m3305_vikhrenko_go/internal/config"
)

type Telemetry struct {
	Logger    *slog.Logger
	Traces    *sdktrace.TracerProvider
	Metrics   *sdkmetric.MeterProvider
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func New(ctx context.Context, cfg config.Config) (*Telemetry, error) {
	logger := NewLogger(cfg.LogLevel, cfg.ServiceName)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { logger.Warn("telemetry export failed", "error", err) }))
	res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithAttributes(attribute.String("service.name", cfg.ServiceName), attribute.String("service.version", "lab-2")))
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}
	traceExporter, metricExporter, err := exporters(ctx, cfg)
	if err != nil {
		return nil, err
	}
	traces := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(res))
	metrics := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)))
	meter := metrics.Meter("tripgo")
	created, err := meter.Int64Counter("tripgo.trips.created", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create trips counter: %w", err)
	}
	completed, err := meter.Int64Counter("tripgo.trips.completed", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create completion counter: %w", err)
	}
	created.Add(ctx, 0)
	completed.Add(ctx, 0)
	return &Telemetry{Logger: logger, Traces: traces, Metrics: metrics, created: created, completed: completed}, nil
}

func exporters(ctx context.Context, cfg config.Config) (sdktrace.SpanExporter, sdkmetric.Exporter, error) {
	var traces sdktrace.SpanExporter
	var metrics sdkmetric.Exporter
	var err error
	if cfg.OTLPProtocol == "grpc" {
		traces, err = otlptracegrpc.New(ctx)
		if err == nil {
			metrics, err = otlpmetricgrpc.New(ctx)
		}
	} else {
		traces, err = otlptracehttp.New(ctx)
		if err == nil {
			metrics, err = otlpmetrichttp.New(ctx)
		}
	}
	if err != nil {
		if traces != nil {
			err = errors.Join(err, traces.Shutdown(ctx))
		}
		return nil, nil, fmt.Errorf("create OTLP exporters: %w", err)
	}
	return traces, metrics, nil
}

func (telemetry *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(telemetry.Metrics.Shutdown(ctx), telemetry.Traces.Shutdown(ctx))
}

func (telemetry *Telemetry) Created(ctx context.Context, id uuid.UUID) {
	telemetry.created.Add(ctx, 1)
	telemetry.Logger.InfoContext(ctx, "trip created", "trip_id", id)
}

func (telemetry *Telemetry) Completed(ctx context.Context, id uuid.UUID) {
	telemetry.completed.Add(ctx, 1)
	telemetry.Logger.InfoContext(ctx, "trip completed", "trip_id", id)
}

func (telemetry *Telemetry) ObserveActive(count func(context.Context) (int64, error)) error {
	meter := telemetry.Metrics.Meter("tripgo")
	gauge, err := meter.Int64ObservableGauge("tripgo.trips.active", metric.WithUnit("{trip}"))
	if err != nil {
		return fmt.Errorf("create active trips gauge: %w", err)
	}
	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		value, err := count(ctx)
		if err != nil {
			telemetry.Logger.WarnContext(ctx, "active trip observation failed", "error", err)
			return nil
		}
		observer.ObserveInt64(gauge, value)
		return nil
	}, gauge)
	return err
}
