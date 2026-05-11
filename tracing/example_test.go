package tracing_test

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/giantswarm/mcp-toolkit/tracing"
)

// ExampleInit_basic shows the typical service composition root.
func ExampleInit_basic() {
	shutdown, err := tracing.Init(context.Background(),
		tracing.WithServiceName("your-mcp"),
		tracing.WithServiceVersion("1.2.3"),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

// ExampleInit_extraResourceAttributes shows how to attach
// deployment-specific attributes (environment, cluster, region) to
// every emitted span without going through OTEL_RESOURCE_ATTRIBUTES.
func ExampleInit_extraResourceAttributes() {
	shutdown, err := tracing.Init(context.Background(),
		tracing.WithServiceName("your-mcp"),
		tracing.WithServiceVersion("1.2.3"),
		tracing.WithResourceOptions(resource.WithAttributes(
			attribute.String("deployment.environment", "production"),
			attribute.String("cluster.name", "glean"),
		)),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}
