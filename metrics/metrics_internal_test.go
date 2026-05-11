package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestInitWithReader_HistogramExemplarAttachesTraceID verifies that
// Init's MeterProvider uses an exemplar filter that attaches the
// active span's TraceID when histogram.Record is called inside a
// sampled span context. This is the mechanism behind Grafana's
// "click latency bucket → jump to trace" pivot.
func TestInitWithReader_HistogramExemplarAttachesTraceID(t *testing.T) {
	// In-memory tracer so we can grab the TraceID we expect to see
	// on the exemplar.
	tracerExp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracerExp))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

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
			h := m.Data.(metricdata.Histogram[float64])
			for _, dp := range h.DataPoints {
				for _, ex := range dp.Exemplars {
					if hexTraceID(ex.TraceID) == wantTraceID {
						sawExemplar = true
					}
				}
			}
		}
	}
	require.True(t, sawExemplar, "expected histogram exemplar carrying the active span's TraceID")
}

func hexTraceID(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, x := range b {
		out[i*2] = hex[x>>4]
		out[i*2+1] = hex[x&0x0f]
	}
	return string(out)
}
