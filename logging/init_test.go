package logging_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/global"

	"github.com/giantswarm/mcp-toolkit/logging"
)

// restoreGlobalLoggerProvider snapshots the global OTel LoggerProvider
// and restores it on cleanup. Init in OTLP mode mutates the global.
func restoreGlobalLoggerProvider(t *testing.T) {
	t.Helper()
	prev := global.GetLoggerProvider()
	t.Cleanup(func() { global.SetLoggerProvider(prev) })
}

func TestInit_OTLP_NoneExporter_ReturnsShutdown(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	// "none" is the no-op autoexport exporter — selects the OTLP code
	// path without actually attempting a network connection.
	t.Setenv("OTEL_LOGS_EXPORTER", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	l, shutdown, err := logging.Init(context.Background(),
		logging.WithServiceName("test-service"),
		logging.WithServiceVersion("0.0.0-test"),
	)
	require.NoError(t, err)
	require.NotNil(t, l)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	l.Info("hello", "k", "v")
	// Use slog.LevelDebug to silence the "v" unused warning in some
	// linter configs — just a sanity call.
	l.Log(context.Background(), slog.LevelInfo, "another")
}
