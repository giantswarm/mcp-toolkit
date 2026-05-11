package metrics_test

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/giantswarm/mcp-toolkit/metrics"
)

// ExampleInit_basic shows the typical service composition root.
func ExampleInit_basic() {
	shutdown, err := metrics.Init(context.Background(),
		metrics.WithServiceName("your-mcp"),
		metrics.WithServiceVersion("1.2.3"),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

// ExampleInit_alwaysOnExemplars shows the override useful in local
// dev or tests: capture an exemplar on every histogram observation,
// not just those inside a sampled span. The production default
// (exemplar.TraceBasedFilter) only attaches exemplars when the
// active SpanContext is sampled.
func ExampleInit_alwaysOnExemplars() {
	shutdown, err := metrics.Init(context.Background(),
		metrics.WithServiceName("your-mcp"),
		metrics.WithExemplarFilter(exemplar.AlwaysOnFilter),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

// ExampleInit_customHistogramBuckets shows how to tune the bucket
// boundaries of a specific histogram instrument. The default
// boundaries are tuned for HTTP-style latency in the 5ms-10s range;
// services with sub-millisecond tool calls need narrower buckets to
// resolve their distribution.
func ExampleInit_customHistogramBuckets() {
	shutdown, err := metrics.Init(context.Background(),
		metrics.WithServiceName("your-mcp"),
		metrics.WithViews(sdkmetric.NewView(
			sdkmetric.Instrument{Name: "your.tool.duration"},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
					Boundaries: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
				},
			},
		)),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

// ExampleInit_extraResourceAttributes shows how to attach
// deployment-specific attributes to every emitted metric without
// going through OTEL_RESOURCE_ATTRIBUTES.
func ExampleInit_extraResourceAttributes() {
	shutdown, err := metrics.Init(context.Background(),
		metrics.WithServiceName("your-mcp"),
		metrics.WithServiceVersion("1.2.3"),
		metrics.WithResourceOptions(resource.WithAttributes(
			attribute.String("deployment.environment", "production"),
			attribute.String("cluster.name", "glean"),
		)),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}
