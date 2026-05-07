package tracing

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown drains any pending spans. Safe to call multiple times.
type Shutdown func(ctx context.Context) error

// Init installs the global OTEL tracer provider and W3C propagator.
//
// The exporter (OTLP/HTTP, OTLP/gRPC, console, or none) is selected by
// autoexport from OTEL_TRACES_EXPORTER + OTEL_EXPORTER_OTLP_PROTOCOL.
// Resource attributes come from process/OS/container detectors merged
// with OTEL_RESOURCE_ATTRIBUTES; render Kubernetes attrs (k8s.pod.name,
// k8s.namespace.name, k8s.node.name) into that env var via the
// downward API.
//
// If neither OTEL_EXPORTER_OTLP_TRACES_ENDPOINT nor
// OTEL_EXPORTER_OTLP_ENDPOINT is set, Init still installs the
// propagator (so inbound traceparent headers propagate) and returns a
// no-op Shutdown.
func Init(ctx context.Context, serviceName, serviceVersion string) (Shutdown, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if !endpointConfigured() {
		return func(context.Context) error { return nil }, nil
	}

	exp, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func endpointConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""
}
