# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [Unreleased]

### Added

- `metrics` package: `metrics.Init(ctx, ...metrics.Option)` returns a `Shutdown` for the OpenTelemetry `MeterProvider`, selecting the exporter via `autoexport.NewMetricReader` from `OTEL_METRICS_EXPORTER`. Supported values: `otlp` (push to a collector), `prometheus` (self-hosted `/metrics` on `OTEL_EXPORTER_PROMETHEUS_HOST:OTEL_EXPORTER_PROMETHEUS_PORT`, default `localhost:9464`), `console`, `none`, or comma-separated combinations (`otlp,prometheus` enables both pipelines from one set of instruments). When no exporter is configured, `Init` returns a no-op `Shutdown` and leaves the SDK's no-op MeterProvider in place. The provider is configured with `exemplar.TraceBasedFilter` explicitly (pinning the current SDK default) so histogram observations recorded inside a sampled span attach the TraceID for Grafana's latency-bucket-to-trace pivot. Mirrors `tracing.Init` and `logging.Init` for API symmetry across the three OTel signals.
- `logging.Init(ctx, ...logging.Option)` returns an `*slog.Logger` plus a `Shutdown` for the OpenTelemetry `LoggerProvider`. When any of `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`, `OTEL_EXPORTER_OTLP_ENDPOINT`, or `OTEL_LOGS_EXPORTER` is set, the primary handler is an `otelslog.Handler` wired to a `BatchProcessor` and an autoexport-selected exporter, and the provider is also registered as the global OTel `LoggerProvider` (parity with `tracing.Init` registering the global `TracerProvider`). Records emitted from inside an active span carry the `SpanContext` (TraceID + SpanID) automatically for log ↔ trace correlation. Otherwise the primary handler is JSON (Kubernetes pod) or text (local) and the `Shutdown` is a no-op. Callers that need the underlying handler — to layer their own subsystem-tag wrapper, redaction handler, etc. — call `Logger.Handler()` on the returned value.
- `logging.WithTraceContextAttrs(slog.Handler) slog.Handler` wraps a stdlib slog handler so emitted records carry `trace_id` and `span_id` slog.Attrs extracted from the context's active `SpanContext`. The OTLP primary picks up trace correlation natively (via the otelslog bridge writing SpanContext to the OTel `LogRecord`'s first-class fields); stdlib `JSONHandler` / `TextHandler` and most third-party handlers ignore ctx for trace data, so extras that need correlation should be wrapped in `WithTraceContextAttrs` before being passed via `logging.WithExtraHandlers`.
- Override `Option` functions on each of the three Init signals (defaults preserved, opt-in):
  - `tracing.WithResourceOptions` (append extra `resource.Option` values for attributes / detectors).
  - `metrics.WithViews` (append `sdkmetric.View` values — rename, drop, override histogram bucket boundaries), `metrics.WithExemplarFilter` (override `exemplar.TraceBasedFilter` — `AlwaysOn` for dev, `AlwaysOff` to disable exemplars; the OTel spec has no `OTEL_METRICS_EXEMPLAR_FILTER` env var at v1.x), `metrics.WithResourceOptions`.
  - `logging.WithExtraHandlers` (fan log records out to additional `slog.Handler` sinks — stderr mirror, file audit, secondary collector), `logging.WithResourceOptions`.

  Speculative options (`tracing.WithSampler`, `tracing.WithPropagators`, `logging.WithBatchProcessorOptions`) intentionally not exposed — add when a real consumer asks. Internal builder is structured so each is a ~10-line addition.
- `internal/otel` package: holds toolkit-private helpers shared across the three signal packages. `otel.Build` composes the SDK Resource from service identity + caller-supplied extras; `otel.Configured` checks the standard OTEL env vars for a given signal (`"traces"` / `"metrics"` / `"logs"`). Not part of the public API surface (Go `internal/` package).

### Changed

- **Breaking:** all three OTel signal `Init` helpers move to a functional-options API for consistency:
  - `tracing.Init(ctx, ...tracing.Option)` (was `Init(ctx, name, version string)`)
  - `metrics.Init(ctx, ...metrics.Option)` (new in this PR)
  - `logging.Init(ctx, ...logging.Option)` returns `(*slog.Logger, Shutdown, error)` (new in this PR)

  Each package exposes an `Option` type and a small set of `WithX` constructors. Defaults are preserved — the common service-composition-root call stays one-line. The `Options` and `InitOptions` structs are removed; the `Format` enum and `FormatAuto` / `FormatText` / `FormatJSON` constants stay.

- **Breaking:** `logging.New` is removed. It was a thin convenience that skipped the `ctx` parameter, the `Shutdown` return, and the `slog.New(handler)` wrap — but at the cost of doubled API surface and a "which one do I use?" decision for new readers. CLI tools and tests now call `logging.Init(context.Background(), ...)` directly; the non-OTLP fallback path is unchanged.

### Fixed

- `tracing.Init`: backport the `exporterOwned` defer pattern from `logging.Init` and `metrics.Init` so a non-partial `resource.New` error after `autoexport.NewSpanExporter` succeeded no longer leaks the exporter's transport (gRPC client, batch goroutine). Closes [#15](https://github.com/giantswarm/mcp-toolkit/issues/15).

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
