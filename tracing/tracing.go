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

// Shutdown drains any pending spans. The second call is a no-op
// (sdktrace.TracerProvider.Shutdown uses sync.Once; the no-endpoint
// branch returns an idempotent closure).
type Shutdown func(ctx context.Context) error

// Init installs the global OTEL tracer provider and W3C propagator.
//
// Init must be called at most once per process. A second call installs
// a new global TracerProvider, leaving the first one's BatchSpanProcessor
// goroutine running with no way to recover a reference for shutdown.
//
// The exporter (OTLP/HTTP, OTLP/gRPC, console, or none) is selected by
// autoexport from OTEL_TRACES_EXPORTER + OTEL_EXPORTER_OTLP_PROTOCOL.
// Resource attributes come from process/OS/container detectors merged
// with OTEL_RESOURCE_ATTRIBUTES; render Kubernetes attrs (k8s.pod.name,
// k8s.namespace.name, k8s.node.name) into that env var via the
// downward API.
//
// If no OTLP endpoint and no OTEL_TRACES_EXPORTER are set, Init still
// installs the propagator (so inbound traceparent headers propagate)
// and returns a no-op Shutdown.
//
// serviceName and serviceVersion are written as semconv attributes only
// when non-empty, so OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES can
// override them.
func Init(ctx context.Context, serviceName, serviceVersion string) (Shutdown, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if !tracingConfigured() {
		return func(context.Context) error { return nil }, nil
	}

	exp, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}
	// Hand exp ownership to the TracerProvider on success; on any
	// error before that handover we must shut it down ourselves or
	// leak its underlying transport (gRPC client, batch goroutine).
	exporterOwned := false
	defer func() {
		if exporterOwned {
			return
		}
		_ = exp.Shutdown(ctx)
	}()

	var attrs []attribute.KeyValue
	if serviceName != "" {
		attrs = append(attrs, semconv.ServiceName(serviceName))
	}
	if serviceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(serviceVersion))
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

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	exporterOwned = true
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// tracingConfigured returns true when either an OTLP endpoint is set or
// an exporter is selected explicitly. Honouring OTEL_TRACES_EXPORTER
// alone covers OTEL_TRACES_EXPORTER=console for local-dev (no endpoint
// needed) and OTEL_TRACES_EXPORTER=none (explicit opt-out via SDK).
func tracingConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_TRACES_EXPORTER") != ""
}
