package tracing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/giantswarm/mcp-toolkit/tracing"
)

// otelEnv is every OTEL_* knob this package or autoexport reads. Tests
// clear them via t.Setenv to isolate from the host environment.
var otelEnv = []string{
	"OTEL_EXPORTER_OTLP_ENDPOINT",
	"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
	"OTEL_EXPORTER_OTLP_PROTOCOL",
	"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL",
	"OTEL_RESOURCE_ATTRIBUTES",
	"OTEL_SERVICE_NAME",
	"OTEL_TRACES_EXPORTER",
}

// resetGlobals must be deferred by every test that calls tracing.Init
// because Init mutates process-global OTel state.
func resetGlobals(t *testing.T) {
	t.Helper()
	for _, k := range otelEnv {
		t.Setenv(k, "")
	}
	t.Cleanup(func() {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	})
}

func TestInit_NoConfig_NoOpButPropagates(t *testing.T) {
	resetGlobals(t)

	shutdown, err := tracing.Init(context.Background(), "svc", "0.0.1")
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()), "first shutdown")
	require.NoError(t, shutdown(context.Background()), "second shutdown must be safe")

	carrier := propagation.MapCarrier{
		"traceparent": "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)
	require.Equal(t,
		"0af7651916cd43dd8448eb211c80319c",
		trace.SpanContextFromContext(ctx).TraceID().String(),
		"propagator must extract incoming traceparent even when no exporter is configured",
	)

	fields := otel.GetTextMapPropagator().Fields()
	require.Contains(t, fields, "traceparent")
	require.Contains(t, fields, "baggage")
}

// TestInit_ExporterConfigured walks the resource builder and
// NewTracerProvider path that runs in production, without standing up a
// collector. OTEL_TRACES_EXPORTER=none yields a noop span exporter via
// autoexport.
func TestInit_ExporterConfigured_InstallsSDKProvider(t *testing.T) {
	resetGlobals(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "none")

	shutdown, err := tracing.Init(context.Background(), "svc", "0.0.1")
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	_, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, ok, "expected SDK TracerProvider, got %T", otel.GetTracerProvider())
}
