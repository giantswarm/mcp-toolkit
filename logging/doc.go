// Package logging provides slog-based logger construction and a
// network-address redaction helper for Giant Swarm MCP servers.
//
// New picks a structured handler — text for local dev, JSON when
// running inside a Kubernetes pod (auto-detected via
// KUBERNETES_SERVICE_HOST).
//
// Init extends New with an OpenTelemetry logs branch: when any of
// OTEL_EXPORTER_OTLP_LOGS_ENDPOINT, OTEL_EXPORTER_OTLP_ENDPOINT, or
// OTEL_LOGS_EXPORTER is set, the returned handler is an
// otelslog.Handler wired to a LoggerProvider that ships records via
// OTLP. Records emitted from within an active span carry the
// SpanContext (TraceID + SpanID) for log ↔ trace correlation in
// Grafana. The Shutdown returned by Init drains the provider on
// graceful exit; in non-OTLP mode it is a no-op closure.
//
// InitOptions.ExtraHandlers attaches additional slog.Handler sinks
// alongside the env-selected primary. Every record reaches the
// primary and each extra, in either OTLP or non-OTLP mode — the
// pattern for "OTLP for Loki plus stderr for kubectl logs" is one
// JSON-on-os.Stderr handler in ExtraHandlers.
//
// Trace correlation differs between the primary and extras: the OTel
// logs SDK attaches SpanContext to its native LogRecord struct as
// first-class fields, so the otelslog-based primary picks up
// TraceID/SpanID without any extra wiring. Stdlib slog handlers
// (JSONHandler, TextHandler) only render the record's attributes —
// they ignore the context's SpanContext. WithTraceContextAttrs wraps
// a handler so the emitted records carry trace_id / span_id keys; use
// it on extras that need correlation:
//
//	stderr := logging.WithTraceContextAttrs(slog.NewJSONHandler(os.Stderr, nil))
//	logging.Init(ctx, logging.InitOptions{
//	    ExtraHandlers: []slog.Handler{stderr},
//	})
//
// RedactHost scrubs IP addresses and URL userinfo before they land in
// logs. It is the only redaction primitive in this package because
// URL parsing plus IPv6 surgery is genuinely non-trivial; format-only
// redactions (e.g. hashing emails, masking tokens) are short enough
// that each server can implement its own opinion at the call site or
// in a custom slog.HandlerOptions.ReplaceAttr.
//
// The slog standard library already provides the LogValuer interface
// for type-driven redaction — see
// https://github.com/golang/go/blob/master/src/log/slog/example_logvaluer_secret_test.go
// — and external libraries such as github.com/m-mizutani/masq cover
// recursive struct walking. The toolkit deliberately does not pre-bake
// either pattern.
package logging
