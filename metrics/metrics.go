package metrics

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown drains pending metric data. Idempotent: the no-exporter
// branch returns a no-op closure and sdkmetric.MeterProvider.Shutdown
// uses sync.Once internally.
type Shutdown func(ctx context.Context) error

// InitOptions configures the MeterProvider. ServiceName and
// ServiceVersion are written as semconv attributes on the Resource
// when non-empty; the standard OTEL_SERVICE_NAME /
// OTEL_RESOURCE_ATTRIBUTES env vars override them.
type InitOptions struct {
	// ServiceName populates semconv.ServiceName on the MeterProvider's
	// Resource. Empty lets OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES
	// own service identity.
	ServiceName string
	// ServiceVersion populates semconv.ServiceVersion. There is no
	// OTEL_SERVICE_VERSION env var; pass the build version here or in
	// OTEL_RESOURCE_ATTRIBUTES.
	ServiceVersion string
}

// Init installs the global OpenTelemetry MeterProvider, selecting the
// exporter via autoexport from OTEL_METRICS_EXPORTER (and the OTLP
// endpoint envs). See the package doc for the full env-var matrix.
//
// When no exporter is configured, Init returns a no-op Shutdown and
// leaves the SDK's no-op MeterProvider in place — consumers can still
// call otel.Meter(...) safely, they just produce nothing.
//
// The returned Shutdown must be deferred by the caller so the
// MeterProvider drains pending data on graceful exit. In the
// autoexport-hosted Prometheus mode, Shutdown also closes the
// /metrics HTTP server.
//
// The MeterProvider is configured with exemplar.TraceBasedFilter
// (which is the SDK default today, but pinning it here insulates
// consumers from future default changes). Histogram observations
// recorded with a context that carries a sampled SpanContext attach
// the active span's TraceID as an exemplar — Grafana's "click latency
// bucket → jump to trace" pivot relies on this.
func Init(ctx context.Context, opts InitOptions) (Shutdown, error) {
	if !metricsConfigured() {
		return func(context.Context) error { return nil }, nil
	}

	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("otel metric reader: %w", err)
	}
	return initWithReader(ctx, reader, opts)
}

// initWithReader constructs the MeterProvider against an explicit
// Reader. Split from Init so tests can inject a ManualReader for
// deterministic record capture; production callers go through Init,
// which selects the Reader via autoexport.
func initWithReader(ctx context.Context, reader sdkmetric.Reader, opts InitOptions) (Shutdown, error) {
	// Hand reader ownership to the MeterProvider on success; on any
	// error before that handover we must shut it down ourselves or
	// leak its underlying transport (Prometheus HTTP server, OTLP
	// gRPC client).
	readerOwned := false
	defer func() {
		if readerOwned {
			return
		}
		_ = reader.Shutdown(ctx)
	}()

	var attrs []attribute.KeyValue
	if opts.ServiceName != "" {
		attrs = append(attrs, semconv.ServiceName(opts.ServiceName))
	}
	if opts.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(opts.ServiceVersion))
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithFromEnv(),
	)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
		sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter),
	)
	readerOwned = true
	otel.SetMeterProvider(mp)
	return mp.Shutdown, nil
}

// metricsConfigured returns true when any of the standard OTEL metric
// env vars opts in. Mirrors the shape of tracing.tracingConfigured
// and logging.otlpLogsConfigured so the three signals follow the
// same env-driven enable pattern.
func metricsConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_METRICS_EXPORTER") != ""
}
