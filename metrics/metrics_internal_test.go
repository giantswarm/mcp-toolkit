package metrics

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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
	shutdown, err := initWithReader(t.Context(), reader, InitOptions{
		ServiceName:    "test-service",
		ServiceVersion: "0.0.0-test",
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

// TestInitWithReader_ExemplarFilter_AlwaysOff verifies that an
// InitOptions.ExemplarFilter override is honoured: AlwaysOff stops the
// MeterProvider from attaching exemplars even when histogram.Record
// fires inside a sampled span context.
func TestInitWithReader_ExemplarFilter_AlwaysOff(t *testing.T) {
	restoreGlobals(t)

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	otel.SetTracerProvider(tp)

	reader := sdkmetric.NewManualReader()
	shutdown, err := initWithReader(t.Context(), reader, InitOptions{
		ServiceName:    "test-service",
		ExemplarFilter: exemplar.AlwaysOffFilter,
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
