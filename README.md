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
