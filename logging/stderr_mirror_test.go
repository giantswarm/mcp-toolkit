package logging_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/logging"
)

func TestInit_WithStderrMirror_OTLPMode_Succeeds(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	t.Setenv("OTEL_LOGS_EXPORTER", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	logger, shutdown, err := logging.Init(context.Background(),
		logging.WithServiceName("test-service"),
		logging.WithStderrMirror(),
	)
	require.NoError(t, err)
	require.NotNil(t, logger)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	// Sanity: emission doesn't panic. The OTel side discards via the
	// "none" exporter; the stderr mirror writes to os.Stderr, which
	// we deliberately don't capture (avoids tampering with the test
	// runner's own stderr).
	logger.Info("hello")
}

func TestInit_WithStderrMirror_NonOTLPMode_Errors(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_LOGS_EXPORTER", "")

	logger, shutdown, err := logging.Init(context.Background(),
		logging.WithStderrMirror(),
	)
	require.Error(t, err, "WithStderrMirror must fail loudly when OTLP isn't configured")
	require.Nil(t, logger)
	require.Nil(t, shutdown)
	require.Contains(t, err.Error(), "WithStderrMirror")
	require.Contains(t, err.Error(), "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
}
