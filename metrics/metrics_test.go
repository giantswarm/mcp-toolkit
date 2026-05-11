package metrics_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"github.com/giantswarm/mcp-toolkit/metrics"
)

// restoreGlobalMeterProvider snapshots the global OTel MeterProvider
// and restores it on cleanup.
func restoreGlobalMeterProvider(t *testing.T) {
	t.Helper()
	prev := otel.GetMeterProvider()
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
}

func TestInit_NoEnv_ReturnsNoopShutdown(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_METRICS_EXPORTER", "")

	shutdown, err := metrics.Init(context.Background())
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()))
	require.NoError(t, shutdown(context.Background()), "Shutdown must be idempotent")
}

func TestInit_ConsoleExporter(t *testing.T) {
	restoreGlobalMeterProvider(t)
	t.Setenv("OTEL_METRICS_EXPORTER", "console")

	shutdown, err := metrics.Init(context.Background(),
		metrics.WithServiceName("test-service"),
		metrics.WithServiceVersion("0.0.0-test"),
	)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() { _ = shutdown(context.Background()) })
}

func TestInit_NoneExporter_ReturnsRealShutdown(t *testing.T) {
	restoreGlobalMeterProvider(t)
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := metrics.Init(context.Background(),
		metrics.WithServiceName("test-service"),
	)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()))
}
