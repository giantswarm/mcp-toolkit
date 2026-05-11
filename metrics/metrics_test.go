package metrics_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/metrics"
)

func TestInit_NoEnv_ReturnsNoopShutdown(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_METRICS_EXPORTER", "")

	shutdown, err := metrics.Init(context.Background(), metrics.InitOptions{})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()))
	require.NoError(t, shutdown(context.Background()), "Shutdown must be idempotent")
}

func TestInit_ConsoleExporter(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "console")

	shutdown, err := metrics.Init(context.Background(), metrics.InitOptions{
		ServiceName:    "test-service",
		ServiceVersion: "0.0.0-test",
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() { _ = shutdown(context.Background()) })
}

func TestInit_NoneExporter_ReturnsRealShutdown(t *testing.T) {
	// OTEL_METRICS_EXPORTER=none exercises the OTLP-mode code path
	// (autoexport returns a no-op reader, but Init still installs a
	// real MeterProvider, sets the global, and returns its Shutdown).
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := metrics.Init(context.Background(), metrics.InitOptions{
		ServiceName: "test-service",
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()))
}
