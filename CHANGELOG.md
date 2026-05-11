# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [Unreleased]

### Added

- `metrics` package: `metrics.Init(ctx, ...metrics.Option) (Shutdown, error)` installs the global OTel `MeterProvider`. Exporter selected via `autoexport.NewMetricReader` from `OTEL_METRICS_EXPORTER`: `otlp`, `prometheus` (self-hosted `/metrics` on `OTEL_EXPORTER_PROMETHEUS_HOST:OTEL_EXPORTER_PROMETHEUS_PORT`, default `localhost:9464`), `console`, `none`, or comma-separated combinations. The provider pins `exemplar.TraceBasedFilter` so histogram observations recorded inside a sampled span attach the TraceID.
- `logging.Init(ctx, ...logging.Option) (*slog.Logger, Shutdown, error)` installs the global OTel `LoggerProvider`. When `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`, `OTEL_EXPORTER_OTLP_ENDPOINT`, or `OTEL_LOGS_EXPORTER` is set, the primary handler is an `otelslog.Handler` (records emitted inside an active span carry `SpanContext` natively). Otherwise the handler is JSON (in-pod, auto-detected via `KUBERNETES_SERVICE_HOST`) or text. Call `Logger.Handler()` to layer a custom handler on top.
- `logging.WithTraceContextAttrs(slog.Handler) slog.Handler` wraps a stdlib slog handler so emitted records carry `trace_id` and `span_id` slog.Attrs extracted from the context's active `SpanContext`. Use on `WithExtraHandlers` sinks that need trace correlation.
- Option constructors:
  - `tracing.WithServiceName`, `WithServiceVersion`, `WithResourceOptions`.
  - `metrics.WithServiceName`, `WithServiceVersion`, `WithViews`, `WithExemplarFilter`, `WithResourceOptions`.
  - `logging.WithFormat`, `WithLevel`, `WithOutput`, `WithLoggerName`, `WithServiceName`, `WithServiceVersion`, `WithExtraHandlers`, `WithResourceOptions`.

  `WithResourceOptions` accepts `resource.Option` values appended to the toolkit defaults (semconv ServiceName/Version, Process, OS, Container, FromEnv). `WithExemplarFilter` accepts `exemplar.AlwaysOnFilter` / `AlwaysOffFilter` / any custom `exemplar.Filter`; there is no `OTEL_METRICS_EXEMPLAR_FILTER` env var in the OTel spec at v1.x.

### Changed

- **Breaking:** `tracing.Init`, `metrics.Init`, `logging.Init` use a functional-options API (`Init(ctx, ...Option)` rather than struct-arg). `tracing.Init`'s prior positional `(name, version string)` signature is replaced by `WithServiceName` / `WithServiceVersion`. The `Options` / `InitOptions` structs are removed; `Format` / `FormatAuto` / `FormatText` / `FormatJSON` stay.
- **Breaking:** `logging.New` is removed. Use `logging.Init(context.Background(), ...)` for the non-OTLP path.

### Fixed

- `tracing.Init`: a non-partial `resource.New` error after `autoexport.NewSpanExporter` succeeded no longer leaks the exporter's transport (gRPC client, batch goroutine). Closes [#15](https://github.com/giantswarm/mcp-toolkit/issues/15).

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
