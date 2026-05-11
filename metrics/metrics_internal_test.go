package metrics

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// restoreGlobals snapshots the global TracerProvider and MeterProvider
// and restores them on cleanup. initWithReader installs a real
// MeterProvider as the global — without this, the first test that
// hits this path leaves a stub provider installed for every test that
// follows.
func restoreGlobals(t *testing.T) {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevMP := otel.GetMeterProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
	})
}

// TestInitWithReader_HistogramExemplarAttachesTraceID verifies that
// Init's MeterProvider uses an exemplar filter that attaches the
// active span's TraceID when histogram.Record is called inside a
// sampled span context. This is the mechanism behind Grafana's
// "click latency bucket → jump to trace" pivot.
func TestInitWithReader_HistogramExemplarAttachesTraceID(t *testing.T) {
	restoreGlobals(t)

	// In-memory tracer so we can grab the TraceID we expect to see
	// on the exemplar.
	tracerExp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracerExp))
	otel.SetTracerProvider(tp)

	// Inject a ManualReader so we can collect synchronously without
	// touching the network.
	reader := sdkmetric.NewManualReader()
	shutdown, err := initWithReader(t.Context(), reader, config{
		serviceName:    "test-service",
		serviceVersion: "0.0.0-test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	tracer := otel.Tracer("github.com/giantswarm/mcp-toolkit/metrics/test")
	ctx, span := tracer.Start(context.Background(), "test-span")

	hist, err := otel.Meter("github.com/giantswarm/mcp-toolkit/metrics/test").
		Float64Histogram("test.duration")
	require.NoError(t, err)
	hist.Record(ctx, 0.123)
	span.End()

	wantTraceID := span.SpanContext().TraceID().String()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var sawExemplar bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "test.duration" {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok, "test.duration must be a float64 Histogram, got %T", m.Data)
			for _, dp := range h.DataPoints {
				for _, ex := range dp.Exemplars {
					if hex.EncodeToString(ex.TraceID) == wantTraceID {
						sawExemplar = true
					}
				}
			}
		}
	}
	require.True(t, sawExemplar, "expected histogram exemplar carrying the active span's TraceID")
}

// TestInitWithReader_WithViews_AppliesCustomHistogramBuckets verifies
// the WithViews override path: a View with custom bucket boundaries
// changes the histogram aggregation for the matching instrument.
func TestInitWithReader_WithViews_AppliesCustomHistogramBuckets(t *testing.T) {
	restoreGlobals(t)

	customBounds := []float64{0.0001, 0.0005, 0.001}
	view := sdkmetric.NewView(
		sdkmetric.Instrument{Name: "test.duration"},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
				Boundaries: customBounds,
			},
		},
	)

	reader := sdkmetric.NewManualReader()
	shutdown, err := initWithReader(t.Context(), reader, config{
		views: []sdkmetric.View{view},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	hist, err := otel.Meter("github.com/giantswarm/mcp-toolkit/metrics/test").
		Float64Histogram("test.duration")
	require.NoError(t, err)
	hist.Record(context.Background(), 0.0007)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var bounds []float64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "test.duration" {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok)
			require.NotEmpty(t, h.DataPoints)
			bounds = h.DataPoints[0].Bounds
		}
	}
	require.Equal(t, customBounds, bounds, "WithViews must override histogram bucket boundaries")
}

// TestInitWithReader_WithResourceOptions_AttachesCallerAttrs verifies
// that caller-supplied resource attributes land on the MeterProvider's
// Resource alongside the toolkit defaults.
func TestInitWithReader_WithResourceOptions_AttachesCallerAttrs(t *testing.T) {
	restoreGlobals(t)

	reader := sdkmetric.NewManualReader()
	shutdown, err := initWithReader(t.Context(), reader, config{
		serviceName: "test-service",
		resourceOptions: []resource.Option{resource.WithAttributes(
			attribute.String("deployment.environment", "production"),
			attribute.String("cluster.name", "glean"),
		)},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	hist, err := otel.Meter("github.com/giantswarm/mcp-toolkit/metrics/test").
		Float64Histogram("test.duration")
	require.NoError(t, err)
	hist.Record(context.Background(), 0.5)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	env, hasEnv := rm.Resource.Set().Value(attribute.Key("deployment.environment"))
	require.True(t, hasEnv, "deployment.environment must be set on the Resource")
	require.Equal(t, "production", env.AsString())
	cluster, hasCluster := rm.Resource.Set().Value(attribute.Key("cluster.name"))
	require.True(t, hasCluster, "cluster.name must be set on the Resource")
	require.Equal(t, "glean", cluster.AsString())

	// Sanity: toolkit defaults still applied.
	svcName, hasSvcName := rm.Resource.Set().Value(attribute.Key("service.name"))
	require.True(t, hasSvcName)
	require.Equal(t, "test-service", svcName.AsString())
}

// TestInitWithReader_ExemplarFilter_AlwaysOff verifies that an
// InitOptions.ExemplarFilter override is honoured: AlwaysOff stops the
// MeterProvider from attaching exemplars even when histogram.Record
// fires inside a sampled span context.
func TestInitWithReader_ExemplarFilter_AlwaysOff(t *testing.T) {
	restoreGlobals(t)

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	otel.SetTracerProvider(tp)

	reader := sdkmetric.NewManualReader()
	shutdown, err := initWithReader(t.Context(), reader, config{
		serviceName:    "test-service",
		exemplarFilter: exemplar.AlwaysOffFilter,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	tracer := otel.Tracer("github.com/giantswarm/mcp-toolkit/metrics/test")
	ctx, span := tracer.Start(context.Background(), "test-span")

	hist, err := otel.Meter("github.com/giantswarm/mcp-toolkit/metrics/test").
		Float64Histogram("test.duration")
	require.NoError(t, err)
	hist.Record(ctx, 0.123)
	span.End()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "test.duration" {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok)
			for _, dp := range h.DataPoints {
				require.Empty(t, dp.Exemplars, "AlwaysOffFilter must suppress all exemplars")
			}
		}
	}
}
