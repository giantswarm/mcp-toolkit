package metrics

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"

	mcptoolkitotel "github.com/giantswarm/mcp-toolkit/internal/otel"
)

// Shutdown drains pending metric data. Idempotent.
type Shutdown func(ctx context.Context) error

// Option configures Init. The zero set of options leaves the SDK's
// no-op MeterProvider in place when no exporter is configured via
// OTEL_*.
type Option func(*config)

type config struct {
	serviceName     string
	serviceVersion  string
	views           []sdkmetric.View
	resourceOptions []resource.Option
	exemplarFilter  exemplar.Filter
}

// WithServiceName sets semconv.ServiceName on the MeterProvider's
// Resource. OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES take
// precedence; empty leaves the env vars in charge.
func WithServiceName(name string) Option {
	return func(c *config) { c.serviceName = name }
}

// WithServiceVersion sets semconv.ServiceVersion. There is no
// OTEL_SERVICE_VERSION env var; pass the build version here or in
// OTEL_RESOURCE_ATTRIBUTES.
func WithServiceVersion(version string) Option {
	return func(c *config) { c.serviceVersion = version }
}

// WithViews appends sdkmetric.View values to the MeterProvider —
// per-instrument customisation: rename, drop, change the aggregation,
// override histogram bucket boundaries. Common case: narrower
// histogram buckets for sub-millisecond tool durations.
func WithViews(views ...sdkmetric.View) Option {
	return func(c *config) { c.views = append(c.views, views...) }
}

// WithResourceOptions appends resource.Option values to the toolkit's
// resource.New option list. Use to add extra attributes
// (deployment.environment, custom labels) or detectors (k8s, AWS,
// GCP). The toolkit's own options — semconv ServiceName/Version,
// Process, OS, Container, FromEnv — are applied first; caller-supplied
// options follow.
func WithResourceOptions(opts ...resource.Option) Option {
	return func(c *config) { c.resourceOptions = append(c.resourceOptions, opts...) }
}

// WithExemplarFilter overrides the default exemplar.TraceBasedFilter
// applied to the MeterProvider. Common overrides:
//
//   - exemplar.AlwaysOnFilter for local dev / tests where there is no
//     tracer but exemplars on every observation are useful for
//     debugging.
//   - exemplar.AlwaysOffFilter to disable exemplars entirely when the
//     metrics backend does not ingest them or exemplar cardinality is
//     a cost concern.
//   - a custom exemplar.Filter for application-specific predicates
//     (e.g. only attach exemplars to slow requests).
//
// There is no OTEL_METRICS_EXEMPLAR_FILTER env var in the OTel spec
// at v1.x, so this option is the only way to deviate from the
// default.
func WithExemplarFilter(filter exemplar.Filter) Option {
	return func(c *config) { c.exemplarFilter = filter }
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
// WithExemplarFilter when the production-correlation default is wrong
// for the deployment.
func Init(ctx context.Context, opts ...Option) (Shutdown, error) {
	c := config{}
	for _, opt := range opts {
		opt(&c)
	}
	if !mcptoolkitotel.Configured("metrics") {
		return func(context.Context) error { return nil }, nil
	}

	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("otel metric reader: %w", err)
	}
	return initWithReader(ctx, reader, c)
}

// initWithReader constructs the MeterProvider against an explicit
// Reader. The seam exists so the Reader is a parameter rather than a
// hidden side effect of autoexport reading the environment.
func initWithReader(ctx context.Context, reader sdkmetric.Reader, c config) (Shutdown, error) {
	readerOwned := false
	defer func() {
		if readerOwned {
			return
		}
		_ = reader.Shutdown(ctx)
	}()

	res, err := mcptoolkitotel.Build(ctx, c.serviceName, c.serviceVersion, c.resourceOptions)
	if err != nil {
		return nil, err
	}

	// Explicit pin: TraceBasedFilter is the SDK default at v1.43.0,
	// but the godoc on WithExemplarFilter reads "SampledFilter".
	// Pinning insulates consumers from default drift if upstream
	// resolves that disagreement in either direction.
	exemplarFilter := c.exemplarFilter
	if exemplarFilter == nil {
		exemplarFilter = exemplar.TraceBasedFilter
	}
	mpOpts := []sdkmetric.Option{
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
		sdkmetric.WithExemplarFilter(exemplarFilter),
	}
	for _, v := range c.views {
		mpOpts = append(mpOpts, sdkmetric.WithView(v))
	}
	mp := sdkmetric.NewMeterProvider(mpOpts...)
	readerOwned = true
	otel.SetMeterProvider(mp)
	return mp.Shutdown, nil
}
