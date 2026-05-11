package tracing

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
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
	propagators     []propagation.TextMapPropagator
	sampler         sdktrace.Sampler
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

// WithPropagators replaces the default TraceContext + Baggage text-map
// propagators on otel.SetTextMapPropagator. Provide every propagator
// the service should support — nothing is appended to the slice.
// Common additions: B3 for legacy systems, Jaeger for vendor
// compatibility.
func WithPropagators(propagators ...propagation.TextMapPropagator) Option {
	return func(c *config) { c.propagators = propagators }
}

// WithSampler overrides the SDK default ParentBased AlwaysSample.
// Set to sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1)) for
// 10% head sampling, or pass a custom sdktrace.Sampler for
// per-request decisions.
func WithSampler(sampler sdktrace.Sampler) Option {
	return func(c *config) { c.sampler = sampler }
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

// Init installs the global OpenTelemetry TracerProvider and the
// text-map propagator.
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

	otel.SetTextMapPropagator(buildPropagators(c.propagators))

	if !tracingConfigured() {
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

	res, err := buildResource(ctx, c)
	if err != nil {
		return nil, err
	}

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	}
	if c.sampler != nil {
		tpOpts = append(tpOpts, sdktrace.WithSampler(c.sampler))
	}
	tp := sdktrace.NewTracerProvider(tpOpts...)
	exporterOwned = true
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// buildPropagators returns the text-map propagator to install: the
// caller's choice when non-empty, otherwise the default
// TraceContext + Baggage composite.
func buildPropagators(custom []propagation.TextMapPropagator) propagation.TextMapPropagator {
	if len(custom) > 0 {
		return propagation.NewCompositeTextMapPropagator(custom...)
	}
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	)
}

// buildResource composes the SDK Resource for the TracerProvider.
// Toolkit defaults run first; caller-supplied ResourceOptions follow.
func buildResource(ctx context.Context, c config) (*resource.Resource, error) {
	var attrs []attribute.KeyValue
	if c.serviceName != "" {
		attrs = append(attrs, semconv.ServiceName(c.serviceName))
	}
	if c.serviceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(c.serviceVersion))
	}
	resourceOpts := []resource.Option{
		resource.WithAttributes(attrs...),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithFromEnv(),
	}
	resourceOpts = append(resourceOpts, c.resourceOptions...)
	res, err := resource.New(ctx, resourceOpts...)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("otel resource: %w", err)
	}
	return res, nil
}

// tracingConfigured returns true when any of the standard OTEL trace
// env vars opts in.
func tracingConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_TRACES_EXPORTER") != ""
}
