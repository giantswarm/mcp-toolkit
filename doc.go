// Package toolkit is the module root of github.com/giantswarm/mcp-toolkit,
// reusable Go building blocks for mcp-go servers extracted from Giant Swarm's
// MCP server fleet. The root package has no API of its own; the modules live
// in the sub-packages:
//
//   - [github.com/giantswarm/mcp-toolkit/middleware/responsecap] and
//     [github.com/giantswarm/mcp-toolkit/middleware/timeout]: tool handler
//     middleware for oversized responses and per-call deadlines
//   - [github.com/giantswarm/mcp-toolkit/health] and
//     [github.com/giantswarm/mcp-toolkit/httpx]: liveness and readiness
//     handlers and a graceful-shutdown wrapper around net/http
//   - [github.com/giantswarm/mcp-toolkit/logging],
//     [github.com/giantswarm/mcp-toolkit/metrics] and
//     [github.com/giantswarm/mcp-toolkit/tracing]: OpenTelemetry signal
//     initialisation driven by the standard OTEL_* environment variables
//
// The root package exists so that the module root is a buildable Go package:
// the CI go-build job compiles the module root, and a root without Go files
// fails with "no Go files".
package toolkit
