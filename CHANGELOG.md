# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [0.2.6](https://github.com/giantswarm/mcp-toolkit/compare/v0.2.5...v0.2.6) (2026-06-03)


### Changed

* **deps:** update dependency architect to v9.1.0 ([#40](https://github.com/giantswarm/mcp-toolkit/issues/40)) ([c2fd6b2](https://github.com/giantswarm/mcp-toolkit/commit/c2fd6b25a69d120747abb7571b87e4fe308e69e7))

## [0.2.5](https://github.com/giantswarm/mcp-toolkit/compare/v0.2.4...v0.2.5) (2026-06-02)


### Changed

* **deps:** update dependency architect to v9.0.2 ([#38](https://github.com/giantswarm/mcp-toolkit/issues/38)) ([8056284](https://github.com/giantswarm/mcp-toolkit/commit/805628460a31b282fdb9cf908bb1fd5af00bafc6))

## [0.2.4](https://github.com/giantswarm/mcp-toolkit/compare/v0.2.3...v0.2.4) (2026-06-02)


### Changed

* **deps:** update dependency architect to v9.0.1 ([#36](https://github.com/giantswarm/mcp-toolkit/issues/36)) ([a6dc64c](https://github.com/giantswarm/mcp-toolkit/commit/a6dc64c83bbf60955381de9ec3e5e0d2d9c01a14))

## [0.2.3](https://github.com/giantswarm/mcp-toolkit/compare/v0.2.2...v0.2.3) (2026-06-01)


### Changed

* **deps:** update dependency architect to v9 ([#32](https://github.com/giantswarm/mcp-toolkit/issues/32)) ([e5e7209](https://github.com/giantswarm/mcp-toolkit/commit/e5e7209341e981f0c01801f401fa9ac3fcbdef89))

## [0.2.2](https://github.com/giantswarm/mcp-toolkit/compare/v0.2.1...v0.2.2) (2026-06-01)


### Fixed

* **deps:** update module github.com/mark3labs/mcp-go to v0.53.0 ([#19](https://github.com/giantswarm/mcp-toolkit/issues/19)) ([1c80117](https://github.com/giantswarm/mcp-toolkit/commit/1c8011768724d742e39449c57152d6db1d52e60e))
* **deps:** update module github.com/mark3labs/mcp-go to v0.54.0 ([#20](https://github.com/giantswarm/mcp-toolkit/issues/20)) ([76a96ae](https://github.com/giantswarm/mcp-toolkit/commit/76a96ae26b66ed5f67188bbde43cc76ac21fcf00))
* **deps:** update module github.com/mark3labs/mcp-go to v0.54.1 ([#26](https://github.com/giantswarm/mcp-toolkit/issues/26)) ([4242699](https://github.com/giantswarm/mcp-toolkit/commit/4242699c17b8b9e9ffbbd0201de4063b113dacf2))
* **deps:** update opentelemetry-go monorepo ([#27](https://github.com/giantswarm/mcp-toolkit/issues/27)) ([a22c19f](https://github.com/giantswarm/mcp-toolkit/commit/a22c19fa910851ddd96f826b2a46061224bb5d6e))
* **deps:** update opentelemetry-go-contrib monorepo ([#29](https://github.com/giantswarm/mcp-toolkit/issues/29)) ([b31b56d](https://github.com/giantswarm/mcp-toolkit/commit/b31b56d21377394dddb1387207adc127c6c1681d))
* **nancy:** remediate nancy findings ([#28](https://github.com/giantswarm/mcp-toolkit/issues/28)) ([492a75b](https://github.com/giantswarm/mcp-toolkit/commit/492a75b163529922c3f2addf8e23fff54dd1618e))
* **release:** seed release-please manifest with current v0.2.1 baseline ([#34](https://github.com/giantswarm/mcp-toolkit/issues/34)) ([d1a1f7e](https://github.com/giantswarm/mcp-toolkit/commit/d1a1f7e4b83adbd697b7943ba72f66d155140c87))


### Changed

* align files according to platform standards ([#25](https://github.com/giantswarm/mcp-toolkit/issues/25)) ([ccb0fdc](https://github.com/giantswarm/mcp-toolkit/commit/ccb0fdcdaaf5593ea73276b98f515b12e26cb550))
* align files according to platform standards ([#30](https://github.com/giantswarm/mcp-toolkit/issues/30)) ([99a5364](https://github.com/giantswarm/mcp-toolkit/commit/99a5364dedec70cad3b2ec0bf12d1acf58a80a8d))
* align files according to platform standards ([#33](https://github.com/giantswarm/mcp-toolkit/issues/33)) ([c6ca00e](https://github.com/giantswarm/mcp-toolkit/commit/c6ca00e5ba46f73d9c947799f1067552108b5fe7))
* **deps:** update dependency architect to v8.1.0 ([#21](https://github.com/giantswarm/mcp-toolkit/issues/21)) ([e8b27ef](https://github.com/giantswarm/mcp-toolkit/commit/e8b27ef57a202274caeb97832226c8703e79cee2))
* **deps:** update dependency architect to v8.2.1 ([#22](https://github.com/giantswarm/mcp-toolkit/issues/22)) ([dadfb86](https://github.com/giantswarm/mcp-toolkit/commit/dadfb866caf56b97d4626c3108f4fc3de4b14027))
* **deps:** update dependency architect to v8.2.2 ([#23](https://github.com/giantswarm/mcp-toolkit/issues/23)) ([ab84fd8](https://github.com/giantswarm/mcp-toolkit/commit/ab84fd8f88ede24e07515df013d5b17d1b58ef9a))
* **deps:** update dependency architect to v8.3.0 ([#24](https://github.com/giantswarm/mcp-toolkit/issues/24)) ([dbb54b2](https://github.com/giantswarm/mcp-toolkit/commit/dbb54b2a38c7305f67708bd0f7615fd081929c8f))

## [Unreleased]

## [0.2.1] - 2026-05-11

### Added

- `logging.WithStderrMirror` option: in OTLP mode, appends a `slog.JSONHandler` on `os.Stderr` (wrapped with `WithTraceContextAttrs` so records carry `trace_id` / `span_id`) to the `ExtraHandlers` list. `Init` returns an error when applied without OTLP logs configured.

## [0.2.0] - 2026-05-11

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

[Unreleased]: https://github.com/giantswarm/mcp-toolkit/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/giantswarm/mcp-toolkit/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/giantswarm/mcp-toolkit/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/giantswarm/mcp-toolkit/releases/tag/v0.1.0
