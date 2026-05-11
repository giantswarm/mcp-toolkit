package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/logging"
)

func TestInit_NoEnv_TextOutput(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_LOGS_EXPORTER", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")

	var buf bytes.Buffer
	h, shutdown, err := logging.Init(context.Background(), logging.InitOptions{
		Options: logging.Options{Format: logging.FormatText, Output: &buf},
	})
	require.NoError(t, err)
	require.NotNil(t, h)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()))
	require.NoError(t, shutdown(context.Background()), "Shutdown must be idempotent")

	slog.New(h).Info("hello", "k", "v")
	require.Contains(t, buf.String(), `msg=hello`)
}

func TestInit_AutoFormat_JSONInsideKubernetes(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_LOGS_EXPORTER", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")

	var buf bytes.Buffer
	h, _, err := logging.Init(context.Background(), logging.InitOptions{
		Options: logging.Options{Output: &buf},
	})
	require.NoError(t, err)

	slog.New(h).Info("hello", "k", "v")
	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, "hello", rec["msg"])
}

func TestInit_OTLP_NoneExporter_ReturnsShutdown(t *testing.T) {
	// "none" is the no-op autoexport exporter — selects the OTLP code
	// path without actually attempting a network connection.
	t.Setenv("OTEL_LOGS_EXPORTER", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	h, shutdown, err := logging.Init(context.Background(), logging.InitOptions{
		Options:        logging.Options{},
		ServiceName:    "test-service",
		ServiceVersion: "0.0.0-test",
	})
	require.NoError(t, err)
	require.NotNil(t, h)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	// Use the handler — emission must not panic and the OTel pipeline
	// must accept the record.
	slog.New(h).Info("hello", "k", "v")
}
