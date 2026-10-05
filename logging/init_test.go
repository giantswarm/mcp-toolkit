package logging_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"github.com/giantswarm/mcp-toolkit/logging"
)

// restoreGlobalLoggerProvider snapshots the global OTel LoggerProvider
// and restores it on cleanup. Init in OTLP mode mutates the global.
func restoreGlobalLoggerProvider(t *testing.T) {
	t.Helper()
	prev := otel.GetLoggerProvider()
	t.Cleanup(func() { otel.SetLoggerProvider(prev) })
}

func TestInit_OTLP_NoneExporter_ReturnsShutdown(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	enableOTLPLogsNone(t)

	l, shutdown, err := logging.Init(context.Background(),
		logging.WithServiceName("test-service"),
		logging.WithServiceVersion("0.0.0-test"),
	)
	require.NoError(t, err)
	require.NotNil(t, l)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	l.Info("hello", "k", "v")
	l.Log(context.Background(), slog.LevelInfo, "another")
}
