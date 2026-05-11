package logging

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// WithTraceContextAttrs wraps h so every Handle call receives the
// active span's TraceID and SpanID as slog.Attrs on the record.
//
// The OTel logs SDK attaches SpanContext to its native LogRecord
// struct as first-class fields, so otelslog.Handler-based pipelines
// carry trace correlation without any extra wiring. Stdlib
// slog.Handler implementations (JSONHandler, TextHandler, and most
// third-party handlers) only render the record's attributes — they
// ignore the context's SpanContext. WithTraceContextAttrs bridges
// that gap: pass a JSON-on-stderr or file handler through it and the
// emitted records carry trace_id / span_id keys the same way the
// OTLP path carries them as native fields.
//
// Records emitted outside an active SpanContext pass through
// unchanged (no attributes added).
func WithTraceContextAttrs(h slog.Handler) slog.Handler {
	return traceAttrHandler{inner: h}
}

type traceAttrHandler struct {
	inner slog.Handler
}

func (h traceAttrHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h traceAttrHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		r = r.Clone()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

func (h traceAttrHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceAttrHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h traceAttrHandler) WithGroup(name string) slog.Handler {
	return traceAttrHandler{inner: h.inner.WithGroup(name)}
}
