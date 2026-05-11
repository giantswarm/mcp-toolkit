package logging_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	mcptoolkitotel "github.com/giantswarm/mcp-toolkit/internal/otel"
	"github.com/giantswarm/mcp-toolkit/logging"
)

// captureStderr swaps os.Stderr for a pipe writer for the duration of
// fn and returns whatever was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })

	done := make(chan string, 1)
	go func() {
		var buf strings.Builder
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

func TestInit_WithStderrMirror_OTLPMode_WritesJSONToStderr(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	enableOTLPLogsNone(t)

	out := captureStderr(t, func() {
		logger, shutdown, err := logging.Init(context.Background(),
			logging.WithServiceName("test-service"),
			logging.WithStderrMirror(),
		)
		require.NoError(t, err)
		require.NotNil(t, logger)
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		logger.Info("hello", "k", "v")
	})

	var rec map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &rec),
		"stderr mirror must emit a JSON record; got: %q", out)
	require.Equal(t, "hello", rec["msg"])
	require.Equal(t, "v", rec["k"])
}

func TestInit_WithStderrMirror_AttachesTraceIDsInsideSpan(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	enableOTLPLogsNone(t)

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	var traceID, spanID string
	out := captureStderr(t, func() {
		logger, shutdown, err := logging.Init(context.Background(),
			logging.WithStderrMirror(),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		ctx, span := tp.Tracer("test").Start(context.Background(), "test-span")
		traceID = span.SpanContext().TraceID().String()
		spanID = span.SpanContext().SpanID().String()
		logger.InfoContext(ctx, "in-span")
		span.End()
	})

	var rec map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &rec),
		"stderr mirror must emit a JSON record; got: %q", out)
	require.Equal(t, traceID, rec["trace_id"], "WithTraceContextAttrs must propagate trace_id to stderr")
	require.Equal(t, spanID, rec["span_id"], "WithTraceContextAttrs must propagate span_id to stderr")
}

func TestInit_WithStderrMirror_NonOTLPMode_Errors(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	clearOTLPEnv(t)

	logger, shutdown, err := logging.Init(context.Background(),
		logging.WithStderrMirror(),
	)
	require.Error(t, err, "WithStderrMirror must fail loudly when OTLP isn't configured")
	require.Nil(t, logger)
	require.Nil(t, shutdown)
	// Each of the three opt-in env vars must be named in the error so
	// a future refactor that silently drops one is caught.
	require.Contains(t, err.Error(), "WithStderrMirror")
	require.Contains(t, err.Error(), mcptoolkitotel.EnvExporterOTLPLogsEndpoint)
	require.Contains(t, err.Error(), mcptoolkitotel.EnvExporterOTLPEndpoint)
	require.Contains(t, err.Error(), mcptoolkitotel.EnvLogsExporter)
}
