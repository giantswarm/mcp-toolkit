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
	// Views customise per-instrument behaviour: rename, drop, change
	// the aggregation, override histogram bucket boundaries, etc.
	// Each view is appended to the MeterProvider in order.
	//
	// Common case — tune the duration histogram buckets for a tool
	// that runs sub-millisecond:
	//
	//	sdkmetric.NewView(
	//	    sdkmetric.Instrument{Name: "your.tool.duration"},
	//	    sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
	//	        Boundaries: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
	//	    }},
	//	)
	Views []sdkmetric.View
	// ResourceOptions are appended to the toolkit's resource.New
	// option list. Use to add extra attributes (deployment.environment,
	// custom labels) or extra detectors (k8s, AWS, GCP). The toolkit's
	// own options — semconv ServiceName/Version, Process, OS,
	// Container, FromEnv — are applied first.
	ResourceOptions []resource.Option
	// ExemplarFilter overrides the default exemplar.TraceBasedFilter
	// applied to the MeterProvider. Common overrides:
	//
	//   - exemplar.AlwaysOnFilter for local dev / tests where there
	//     is no tracer but exemplars on every observation are useful
	//     for debugging.
	//   - exemplar.AlwaysOffFilter to disable exemplars entirely
	//     when the metrics backend does not ingest them or exemplar
	//     cardinality is a cost concern.
	//   - a custom exemplar.Filter for application-specific
	//     predicates (e.g. only attach exemplars to slow requests).
	//
	// There is no OTEL_METRICS_EXEMPLAR_FILTER env var in the OTel
	// spec at v1.x, so configuration via this field is the only way
	// to deviate from the default.
	//
	// Nil keeps exemplar.TraceBasedFilter: attach the active span's
	// TraceID when the SpanContext is sampled. Suitable for
	// production Grafana / Mimir + Tempo correlation.
	ExemplarFilter exemplar.Filter
}

// Init installs the global OpenTelemetry MeterProvider, selecting the
// exporter via autoexport from OTEL_METRICS_EXPORTER (and the OTLP
// endpoint envs). See the package doc for the full env-var matrix.
//
// Init must be called at most once per process. A second call installs
// a new global MeterProvider, leaving the first one's Reader goroutine
// (and Prometheus HTTP server, in prometheus mode) running with no way
// to recover a reference for shutdown.
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
// The MeterProvider is configured with exemplar.TraceBasedFilter by
// default — the same value as the SDK default at v1.43.0, pinned
// here to insulate consumers from future SDK drift. Override via
// InitOptions.ExemplarFilter when the production-correlation default
// is wrong for the deployment.
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
// Reader. The seam exists so the Reader is a parameter rather than a
// hidden side effect of autoexport reading the environment.
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

	res, err := buildResource(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Explicit pin: TraceBasedFilter is the SDK default at v1.43.0,
	// but the godoc on WithExemplarFilter reads "SampledFilter".
	// Pinning insulates consumers from default drift if upstream
	// resolves that disagreement in either direction.
	exemplarFilter := opts.ExemplarFilter
	if exemplarFilter == nil {
		exemplarFilter = exemplar.TraceBasedFilter
	}
	mpOpts := []sdkmetric.Option{
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
		sdkmetric.WithExemplarFilter(exemplarFilter),
	}
	for _, v := range opts.Views {
		mpOpts = append(mpOpts, sdkmetric.WithView(v))
	}
	mp := sdkmetric.NewMeterProvider(mpOpts...)
	readerOwned = true
	otel.SetMeterProvider(mp)
	return mp.Shutdown, nil
}

// buildResource composes the SDK Resource for the MeterProvider.
// Toolkit defaults run first; caller-supplied ResourceOptions follow.
func buildResource(ctx context.Context, opts InitOptions) (*resource.Resource, error) {
	var attrs []attribute.KeyValue
	if opts.ServiceName != "" {
		attrs = append(attrs, semconv.ServiceName(opts.ServiceName))
	}
	if opts.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(opts.ServiceVersion))
	}
	resourceOpts := []resource.Option{
		resource.WithAttributes(attrs...),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithFromEnv(),
	}
	resourceOpts = append(resourceOpts, opts.ResourceOptions...)
	res, err := resource.New(ctx, resourceOpts...)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("otel resource: %w", err)
	}
	return res, nil
}

// metricsConfigured returns true when any of the standard OTEL metric
// env vars opts in.
func metricsConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_METRICS_EXPORTER") != ""
}
