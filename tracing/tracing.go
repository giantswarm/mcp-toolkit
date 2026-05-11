package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	mcptoolkitotel "github.com/giantswarm/mcp-toolkit/internal/otel"
)

// Shutdown drains any pending spans. Idempotent.
type Shutdown func(ctx context.Context) error

// Option configures Init. The zero set of options gives an
// unconfigured service: the W3C propagator is installed and a no-op
// Shutdown is returned.
type Option func(*config)

type config struct {
	serviceName     string
	serviceVersion  string
	resourceOptions []resource.Option
}

// WithServiceName sets semconv.ServiceName on the TracerProvider's
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

// WithResourceOptions appends resource.Option values to the toolkit's
// resource.New option list. Use to add extra attributes
// (deployment.environment, custom labels) or detectors (k8s, AWS,
// GCP). The toolkit's own options — semconv ServiceName/Version,
// Process, OS, Container, FromEnv — are applied first; caller-supplied
// options follow.
func WithResourceOptions(opts ...resource.Option) Option {
	return func(c *config) { c.resourceOptions = append(c.resourceOptions, opts...) }
}

// Init installs the global OpenTelemetry TracerProvider and the W3C
// TraceContext + Baggage propagators.
//
// Init must be called at most once per process. A second call installs
// a new global TracerProvider, leaving the first one's
// BatchSpanProcessor goroutine running with no way to recover a
// reference for shutdown.
//
// The exporter (OTLP/HTTP, OTLP/gRPC, console, or none) is selected by
// autoexport from OTEL_TRACES_EXPORTER + OTEL_EXPORTER_OTLP_PROTOCOL.
// Resource attributes come from process/OS/container detectors merged
// with OTEL_RESOURCE_ATTRIBUTES and any caller-supplied
// WithResourceOptions; render Kubernetes attrs (k8s.pod.name,
// k8s.namespace.name, k8s.node.name) into OTEL_RESOURCE_ATTRIBUTES via
// the downward API.
//
// If no OTLP endpoint and no OTEL_TRACES_EXPORTER are set, Init still
// installs the propagator (so inbound traceparent headers propagate)
// and returns a no-op Shutdown.
func Init(ctx context.Context, opts ...Option) (Shutdown, error) {
	c := config{}
	for _, opt := range opts {
		opt(&c)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if !mcptoolkitotel.Configured("traces") {
		return func(context.Context) error { return nil }, nil
	}

	exp, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}
	exporterOwned := false
	defer func() {
		if exporterOwned {
			return
		}
		_ = exp.Shutdown(ctx)
	}()

	res, err := mcptoolkitotel.Build(ctx, c.serviceName, c.serviceVersion, c.resourceOptions)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	exporterOwned = true
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

