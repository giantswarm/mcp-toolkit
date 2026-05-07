package tracing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/giantswarm/mcp-toolkit/tracing"
)

func clearEndpoint(t *testing.T) {
	t.Helper()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
}

func TestInit_NoEndpoint_StillPropagates(t *testing.T) {
	clearEndpoint(t)

	shutdown, err := tracing.Init(context.Background(), "svc", "0.0.1")
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))

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
