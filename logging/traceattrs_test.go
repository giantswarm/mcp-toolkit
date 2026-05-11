package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/giantswarm/mcp-toolkit/logging"
)

func TestWithTraceContextAttrs_AddsTraceIDAndSpanIDInsideSpan(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	tracer := tp.Tracer("github.com/giantswarm/mcp-toolkit/logging/test")

	var buf bytes.Buffer
	h := logging.WithTraceContextAttrs(slog.NewJSONHandler(&buf, nil))

	ctx, span := tracer.Start(context.Background(), "test-span")
	slog.New(h).InfoContext(ctx, "hello")
	span.End()

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, span.SpanContext().TraceID().String(), rec["trace_id"])
	require.Equal(t, span.SpanContext().SpanID().String(), rec["span_id"])
}

func TestWithTraceContextAttrs_PassesThroughOutsideSpan(t *testing.T) {
	var buf bytes.Buffer
	h := logging.WithTraceContextAttrs(slog.NewJSONHandler(&buf, nil))

	slog.New(h).InfoContext(context.Background(), "hello")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	_, hasTrace := rec["trace_id"]
	_, hasSpan := rec["span_id"]
	require.False(t, hasTrace, "no trace_id should be added when ctx has no span")
	require.False(t, hasSpan, "no span_id should be added when ctx has no span")
}
