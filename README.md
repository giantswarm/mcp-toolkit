# mcp-toolkit

Reusable Go policies and native adapters for [mcp-go](https://github.com/mark3labs/mcp-go) and the [official Go SDK](https://github.com/modelcontextprotocol/go-sdk).

[![Go reference](https://pkg.go.dev/badge/github.com/giantswarm/mcp-toolkit.svg)](https://pkg.go.dev/github.com/giantswarm/mcp-toolkit)
[![Go report card](https://goreportcard.com/badge/github.com/giantswarm/mcp-toolkit)](https://goreportcard.com/report/github.com/giantswarm/mcp-toolkit)

## Status

Early. APIs may shift until the first set of middleware has been adopted by two or more consumers in production.

## Scope

A home for shared tool policy and small helpers extracted from MCP servers. Protocol negotiation, registration, elicitation, errors and results stay native to each SDK. The application owns configuration, authorization, transports and process lifecycle.

Successful patterns from this module are upstream candidates for `mark3labs/mcp-go` once they have settled.

## Modules

| Path | Purpose |
|---|---|
| [`k8s`](./k8s) (separate Go module) | Caller-bound client-go clients and SelfSubjectAccessReviews, with explicit transport wrappers and expiry-aware caching. |
| [`toolpolicy`](./toolpolicy) | Resolved limits, a validated identity accessor, logger and meter for both adapters. Audit includes issuer, subject, actor, tool and outcome, with no profile, argument or credential values. |
| [`middleware/mcpgo`](./middleware/mcpgo) | Native mcp-go server option using its complete-result size limit, plus shared audit, deadlines and `mcp.server.operation.duration`. |
| [`middleware/gosdk`](./middleware/gosdk) | Official SDK handler decorator with the same policy and a complete JSON result limit. |
| [`middleware/responsecap`](./middleware/responsecap) | Reject oversized tool responses with a typed `response_too_large` error and `IsError = true`, instead of letting the LLM consume truncated-but-syntactically-valid output. |
| [`middleware/timeout`](./middleware/timeout) | Per-tool-call `context.WithTimeout` middleware. On deadline, returns an `IsError` `CallToolResult` containing `tool X exceeded timeout of Ys` rather than a silent hang or generic context error. Parent-context cancellation propagates unchanged. |
| [`health`](./health) | Stdlib-only `/healthz` (unconditional 200) and `/readyz` (atomic `SetReady` flag) HTTP handlers. Liveness can't flap; readiness is pod-local so downstream hiccups don't yank every replica's endpoint at once. |
| [`httpx`](./httpx) | Graceful HTTP shutdown, RFC 9457 `Problem` / `WriteProblem`, and `ProtectOrigin` for every MCP HTTP method, including loopback Host validation. |
| [`logging`](./logging) | `slog.Logger` factory that auto-picks text vs JSON based on `KUBERNETES_SERVICE_HOST`, plus a `RedactHost` helper that scrubs IPs and URL userinfo from log strings. `logging.Init` adds an OpenTelemetry logs branch (`otelslog` bridge over an OTLP exporter) for log↔trace correlation when `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` / `OTEL_LOGS_EXPORTER` is set. |
| [`metrics`](./metrics) | OpenTelemetry meter-provider init driven by `OTEL_METRICS_EXPORTER` via autoexport — `otlp` (push to a collector), `prometheus` (self-hosted `/metrics` on `localhost:9464`), `console`, or `none`. Comma-separated values enable multiple exporters from one set of instruments. Pins `exemplar.TraceBasedFilter` so histogram observations recorded inside a sampled span attach the TraceID for Grafana's latency-bucket-to-trace pivot. |
| [`tracing`](./tracing) | OpenTelemetry tracer-provider init driven by standard `OTEL_*` env vars. Always installs W3C TraceContext + Baggage propagators (so inbound `traceparent` headers chain) and returns a no-op shutdown when no exporter is configured. |
| [`tooldesc`](./tooldesc) | Tool descriptions that lead with a summary line of at most 120 runes, all muster's `filter_tools` shows: `Describe(summary, details)` panics on a summary that doesn't fit as the tool is built, so the server's tests fail; `CheckServer` holds every description, and every description in the input schemas, to a budget in a test, and `RenderServerDoc` writes the `docs/tools.md` reviewers read the text in, which `tooldesctest.AssertServerDoc` keeps current in a test. What goes where is in [`docs/conventions.md`](./docs/conventions.md#tool-descriptions). |
| [`brand`](./brand) | The Giant Swarm logo as the server's MCP icon (`serverInfo.icons`), embedded as an SVG data URI so clients such as muster and the developer portal can show it without network access: `server.NewMCPServer("my-mcp", "1.0.0", brand.WithGiantSwarmIcon())`. `brand.WithIcon(svg)` sets another embedded SVG, such as an upstream project's logo. |
| [`brand/gosdk`](./brand/gosdk), [`tooldesc/gosdk`](./tooldesc/gosdk) | Native official SDK icons, description budgets and documentation rendering without an mcp-go dependency. |

Conventions consumer MCP servers should follow are documented in [`docs/conventions.md`](./docs/conventions.md) — the paginated tool-result shape (`{ items, nextCursor }`) and what goes into a tool description.

More to follow as they get extracted from real consumers.

## Usage

Each package has its own documentation. Configure policy at server construction:

```go
import (
    mcpserver "github.com/mark3labs/mcp-go/server"
    policy "github.com/giantswarm/mcp-toolkit/middleware/mcpgo"
    "github.com/giantswarm/mcp-toolkit/toolpolicy"
)

s := mcpserver.NewMCPServer("my-mcp", "1.0.0",
    policy.New(toolpolicy.Options{MaxResultBytes: 128 << 10, Logger: logger}),
)
```

For the official SDK, wrap each native handler with `middleware/gosdk.New(options)` before `Server.AddTool`. `Timeout == 0` selects 30 seconds; negative disables it. Nonpositive `MaxResultBytes` disables the limit. Applications whose existing configuration uses zero to disable timeouts translate it to a negative duration. Both adapters distinguish parent cancellation from their own deadline and leave native SDK spans intact. The older `middleware/timeout` and text-only `middleware/responsecap` remain available during consumer migration.

The OTel signal `Init` helpers — `tracing.Init`, `metrics.Init`,
`logging.Init` — share a functional-options API. The common shape is
one call per signal at the service composition root, with each option
opting into a specific OTel SDK knob:

```go
import (
    "github.com/giantswarm/mcp-toolkit/logging"
    "github.com/giantswarm/mcp-toolkit/metrics"
    "github.com/giantswarm/mcp-toolkit/tracing"
)

ctx := context.Background()

shutdownTracing, err := tracing.Init(ctx,
    tracing.WithServiceName("my-mcp"),
    tracing.WithServiceVersion("1.2.3"),
)
// handle err; defer shutdownTracing(ctx)

shutdownMetrics, err := metrics.Init(ctx,
    metrics.WithServiceName("my-mcp"),
    metrics.WithServiceVersion("1.2.3"),
)
// handle err; defer shutdownMetrics(ctx)

logger, shutdownLogging, err := logging.Init(ctx,
    logging.WithServiceName("my-mcp"),
    logging.WithServiceVersion("1.2.3"),
    logging.WithLoggerName("github.com/your/repo"),
)
// handle err; defer shutdownLogging(ctx); slog.SetDefault(logger)
```

Override knobs are package-local `Option` functions:
`metrics.WithViews`, `metrics.WithExemplarFilter`,
`logging.WithExtraHandlers`, `logging.WithTraceContextAttrs`,
`*.WithResourceOptions`. Each package's runnable `Example_*` functions
show idiomatic combinations.

## Contributing

Before adding a module, it should be in use by at least one Giant Swarm MCP server. Speculative middleware does not belong here — extract from the real call site, not the other way around.

## License

Apache 2.0. See [LICENSE](LICENSE).
