package tracing_test

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/giantswarm/mcp-toolkit/tracing"
)

// ExampleInit_basic shows the typical service composition root.
func ExampleInit_basic() {
	shutdown, err := tracing.Init(context.Background(), tracing.InitOptions{
		ServiceName:    "your-mcp",
		ServiceVersion: "1.2.3",
	})
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

// ExampleInit_customPropagators shows how to add a non-default
// propagator alongside the W3C TraceContext + Baggage — typical when
// interoperating with services that emit B3 (via
// go.opentelemetry.io/contrib/propagators/b3) or Jaeger headers. The
// Propagators slice replaces the default composite, so include every
// propagator the service should support.
func ExampleInit_customPropagators() {
	shutdown, err := tracing.Init(context.Background(), tracing.InitOptions{
		ServiceName: "your-mcp",
		Propagators: []propagation.TextMapPropagator{
			propagation.TraceContext{},
			propagation.Baggage{},
			// b3.New(), // add when go.opentelemetry.io/contrib/propagators/b3 is imported
		},
	})
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

// ExampleInit_headSampling shows ratio-based head sampling. Production
// services with high request rates often drop the default
// AlwaysSample to keep span volume manageable.
func ExampleInit_headSampling() {
	shutdown, err := tracing.Init(context.Background(), tracing.InitOptions{
		ServiceName: "your-mcp",
		Sampler:     sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1)), // 10% head sample
	})
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

// ExampleInit_extraResourceAttributes shows how to attach
// deployment-specific attributes (environment, cluster, region) to
// every emitted span without going through OTEL_RESOURCE_ATTRIBUTES.
func ExampleInit_extraResourceAttributes() {
	shutdown, err := tracing.Init(context.Background(), tracing.InitOptions{
		ServiceName:    "your-mcp",
		ServiceVersion: "1.2.3",
		ResourceOptions: []resource.Option{
			resource.WithAttributes(
				attribute.String("deployment.environment", "production"),
				attribute.String("cluster.name", "glean"),
			),
		},
	})
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}
