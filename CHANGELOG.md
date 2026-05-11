# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [Unreleased]

### Added

- `logging.Init` returns a `slog.Handler` plus a `Shutdown` for the OpenTelemetry `LoggerProvider`. When any of `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`, `OTEL_EXPORTER_OTLP_ENDPOINT`, or `OTEL_LOGS_EXPORTER` is set, the handler is an `otelslog.Handler` wired to a `BatchProcessor` and an autoexport-selected exporter, and the provider is also registered as the global OTel `LoggerProvider` (parity with `tracing.Init` registering the global `TracerProvider`). Records emitted from inside an active span carry the `SpanContext` (TraceID + SpanID) automatically for log ↔ trace correlation. Otherwise the handler is the same text/JSON one `New` produces, and `Shutdown` is a no-op. The OTLP path requires neither traces nor metrics to be configured — the three signals are independent. Use `New` when no `Shutdown` is needed (no OTLP, no `LoggerProvider` lifecycle); use `Init` from a service composition root that wants OTLP logs.
- `logging.InitOptions` embeds `Options` and adds `LoggerName`, `ServiceName`, `ServiceVersion`. `LoggerName` identifies the instrumentation scope on every emitted `LogRecord` (conventionally the consumer's module path, e.g. `github.com/giantswarm/muster`); `ServiceName` / `ServiceVersion` populate `semconv.ServiceName` / `semconv.ServiceVersion` on the `LoggerProvider`'s `Resource`. All three honour the standard env-var overrides (`OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`). The non-OTLP path ignores them: pod / container / namespace identity comes from log-collector stream metadata.

## [0.1.0] - 2026-05-07

### Added

- `middleware/responsecap` package: `responsecap.New` returns a `server.ToolHandlerMiddleware` that rejects oversized `TextContent` entries in `CallToolResult` with a typed `response_too_large` error and `IsError = true`, instead of letting the LLM consume truncated-but-syntactically-valid output. `Meta` and non-text content are left untouched; truncation is intentionally not offered.
- `middleware/timeout` package: `timeout.New` returns a `server.ToolHandlerMiddleware` that wraps each tool call in `context.WithTimeout` and replaces deadline expiry with an `IsError` `CallToolResult` carrying `tool X exceeded timeout of Ys` — so the LLM sees actionable text instead of a silent hang or a generic context error. Parent-context cancellation propagates unchanged so upstream cancellations are not masqueraded as toolkit timeouts.
- `health` package: stdlib-only `Health` type that serves `/healthz` (unconditional 200) and `/readyz` (200/503 driven by an atomic `SetReady` flag). Liveness is intentionally static so it cannot flap; readiness is pod-local so a transient downstream hiccup cannot flip every replica's `/readyz` at once. Servers that already pull in `controller-runtime` should prefer its `pkg/healthz` primitives instead.
- `httpx` package: `httpx.Run` is a small graceful-shutdown wrapper around `net/http`. Starts `srv.ListenAndServe` in a goroutine and blocks until the parent context is canceled (then calls `srv.Shutdown` with the configured timeout) or the server returns an error. Deliberately does not abstract listener choice — TLS or custom listeners stay in caller code.
- `logging` package: `logging.New` builds a `*slog.Logger` whose handler is auto-selected — text for local dev, JSON when running inside a Kubernetes pod (auto-detected via `KUBERNETES_SERVICE_HOST`). `logging.RedactHost` scrubs IP addresses and URL userinfo before they land in logs; format-only redactions (hashing emails, masking tokens) are left to call sites or to slog's `LogValuer` / `ReplaceAttr` hooks.
- `tracing` package: `tracing.Init` installs an OpenTelemetry tracer provider configured from the standard `OTEL_*` environment variables, with W3C TraceContext + Baggage propagation. Returns a no-op shutdown when neither an OTLP endpoint nor `OTEL_TRACES_EXPORTER` is set, but always installs the propagator so inbound `traceparent` headers continue to chain. Note: depends on `go.opentelemetry.io/contrib/exporters/autoexport`, which transitively pulls in the OTel exporter constellation (Prometheus bridge, stdoutlog, otlp metric/log exporters) — accepted in exchange for OTel-maintained protocol selection.
- `docs/conventions.md`: project-wide conventions consumer MCP servers should follow even when no toolkit code enforces them. Documents the paginated tool-result shape (`{ items, nextCursor }`, camelCase to match `mcp.PaginatedResult.NextCursor`).

[Unreleased]: https://github.com/giantswarm/mcp-toolkit/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/giantswarm/mcp-toolkit/releases/tag/v0.1.0
