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

// InitOptions configures Init. The zero value is usable for an
// unconfigured service — it installs only the W3C propagator and
// returns a no-op Shutdown.
type InitOptions struct {
	// ServiceName populates semconv.ServiceName on the
	// TracerProvider's Resource when non-empty. Empty lets
	// OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES own service
	// identity.
	ServiceName string
	// ServiceVersion populates semconv.ServiceVersion. There is no
	// OTEL_SERVICE_VERSION env var; pass the build version here or in
	// OTEL_RESOURCE_ATTRIBUTES.
	ServiceVersion string
	// Propagators overrides the default
	// (TraceContext + Baggage) text-map propagators. Provide the full
	// list — nothing is appended to it. Common additions: B3 for
	// legacy systems, Jaeger for vendor compatibility. Empty falls
	// back to the default.
	Propagators []propagation.TextMapPropagator
	// Sampler overrides the SDK default (ParentBased AlwaysSample).
	// Set to a sdktrace.TraceIDRatioBased for head-based sampling, or
	// a custom sdktrace.Sampler for per-request decisions. Nil keeps
	// the default.
	Sampler sdktrace.Sampler
	// ResourceOptions are appended to the toolkit's resource.New
	// option list. Use to add extra attributes (deployment.environment,
	// custom labels) or extra detectors (k8s, AWS, GCP). The toolkit's
	// own options — semconv ServiceName/Version, Process, OS,
	// Container, FromEnv — are applied first so caller-supplied
	// attributes can override them where the SDK respects
	// last-write semantics.
	ResourceOptions []resource.Option
}

// Init installs the global OpenTelemetry TracerProvider and the W3C
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
// InitOptions.ResourceOptions; render Kubernetes attrs (k8s.pod.name,
// k8s.namespace.name, k8s.node.name) into OTEL_RESOURCE_ATTRIBUTES via
// the downward API.
//
// If no OTLP endpoint and no OTEL_TRACES_EXPORTER are set, Init still
// installs the propagator (so inbound traceparent headers propagate)
// and returns a no-op Shutdown.
func Init(ctx context.Context, opts InitOptions) (Shutdown, error) {
	otel.SetTextMapPropagator(propagators(opts.Propagators))

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

	res, err := buildResource(ctx, opts)
	if err != nil {
		return nil, err
	}

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	}
	if opts.Sampler != nil {
		tpOpts = append(tpOpts, sdktrace.WithSampler(opts.Sampler))
	}
	tp := sdktrace.NewTracerProvider(tpOpts...)
	exporterOwned = true
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// propagators returns the text-map propagator to install: the
// caller's choice when non-empty, otherwise the default
// TraceContext + Baggage composite.
func propagators(custom []propagation.TextMapPropagator) propagation.TextMapPropagator {
	if len(custom) > 0 {
		return propagation.NewCompositeTextMapPropagator(custom...)
	}
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	)
}

// buildResource composes the SDK Resource for the TracerProvider.
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

// tracingConfigured returns true when any of the standard OTEL trace
// env vars opts in.
func tracingConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_TRACES_EXPORTER") != ""
}
