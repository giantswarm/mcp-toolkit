# mcp-toolkit

Reusable Go middleware for [mcp-go](https://github.com/mark3labs/mcp-go) servers, extracted from Giant Swarm's MCP server fleet.

[![Go reference](https://pkg.go.dev/badge/github.com/giantswarm/mcp-toolkit.svg)](https://pkg.go.dev/github.com/giantswarm/mcp-toolkit)
[![Go report card](https://goreportcard.com/badge/github.com/giantswarm/mcp-toolkit)](https://goreportcard.com/report/github.com/giantswarm/mcp-toolkit)

## Status

Early. APIs may shift until the first set of middleware has been adopted by two or more consumers in production.

## Scope

A home for `server.ToolHandlerMiddleware` implementations and small helpers we found ourselves rewriting across MCP servers (`mcp-prometheus`, `mcp-observability-platform`, `mcp-kubernetes`, …). Anything generic enough to live next to mcp-go's own `output_validation.go` is fair game; anything specific to one MCP stays in that MCP.

Successful patterns from this module are upstream candidates for `mark3labs/mcp-go` once they have settled.

## Modules

| Path | Purpose |
|---|---|
| [`middleware/responsecap`](./middleware/responsecap) | Reject oversized tool responses with a typed `response_too_large` error and `IsError = true`, instead of letting the LLM consume truncated-but-syntactically-valid output. |
| [`middleware/timeout`](./middleware/timeout) | Per-tool-call `context.WithTimeout` middleware. On deadline, returns an `IsError` `CallToolResult` containing `tool X exceeded timeout of Ys` rather than a silent hang or generic context error. Parent-context cancellation propagates unchanged. |
| [`health`](./health) | Stdlib-only `/healthz` (unconditional 200) and `/readyz` (atomic `SetReady` flag) HTTP handlers. Liveness can't flap; readiness is pod-local so downstream hiccups don't yank every replica's endpoint at once. |
| [`httpx`](./httpx) | Graceful-shutdown wrapper around `net/http`: `Run` starts `ListenAndServe` and blocks until ctx cancel (then calls `Shutdown` with a configured timeout) or the server returns an error. |
| [`logging`](./logging) | `slog.Logger` factory that auto-picks text vs JSON based on `KUBERNETES_SERVICE_HOST`, plus a `RedactHost` helper that scrubs IPs and URL userinfo from log strings. `logging.Init` adds an OpenTelemetry logs branch (`otelslog` bridge over an OTLP exporter) for log↔trace correlation when `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` / `OTEL_LOGS_EXPORTER` is set. |
| [`metrics`](./metrics) | OpenTelemetry meter-provider init driven by `OTEL_METRICS_EXPORTER` via autoexport — `otlp` (push to a collector), `prometheus` (self-hosted `/metrics` on `localhost:9464`), `console`, or `none`. Comma-separated values enable multiple exporters from one set of instruments. Pins `exemplar.TraceBasedFilter` so histogram observations recorded inside a sampled span attach the TraceID for Grafana's latency-bucket-to-trace pivot. |
| [`tracing`](./tracing) | OpenTelemetry tracer-provider init driven by standard `OTEL_*` env vars. Always installs W3C TraceContext + Baggage propagators (so inbound `traceparent` headers chain) and returns a no-op shutdown when no exporter is configured. |

Conventions consumer MCP servers should follow are documented in [`docs/conventions.md`](./docs/conventions.md) — currently the paginated tool-result shape (`{ items, nextCursor }`).

More to follow as they get extracted from real consumers.

## Usage

Each module has its own package documentation. The general shape:

```go
import (
    mcpserver "github.com/mark3labs/mcp-go/server"
    "github.com/giantswarm/mcp-toolkit/middleware/responsecap"
)

s := mcpserver.NewMCPServer("my-mcp", "1.0.0")
s.Use(responsecap.New(responsecap.Options{Limit: 128 << 10}))
```

## Contributing

Before adding a module, it should be in use by at least one Giant Swarm MCP server. Speculative middleware does not belong here — extract from the real call site, not the other way around.

## License

Apache 2.0. See [LICENSE](LICENSE).
