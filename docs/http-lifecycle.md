# HTTP health and shutdown

The `health` and `httpx` packages use only the standard library and work with
both mark3labs/mcp-go and modelcontextprotocol/go-sdk. They do not change
protocol negotiation, authorization or SDK error representations.

## Probes

`health.New` starts not ready. Mount its handlers on an `http.ServeMux` and
set readiness after initialization completes. Mounted routes accept GET and
HEAD, reject other methods with 405, and disable response caching.

Liveness checks HTTP responsiveness. It cannot detect a stalled worker that
leaves the HTTP handler responsive. Readiness describes whether useful work
can be accepted. Dependency checks are application-specific: a shared backend
outage should not withdraw every replica if useful requests can still be served.
Probe bodies contain status text rather than operational errors or credentials.

These semantics follow [Kubernetes probe guidance](https://kubernetes.io/docs/concepts/workloads/pods/probes/).
They are operational conventions, not OAuth, MCP or A2A protocol requirements.
Applications using controller-runtime can retain its health and lifecycle support.

## Connection draining

`httpx.Run` drains on context cancellation and returns a shutdown timeout
without closing active connections. Consumers that need bounded connection
lifetimes can opt in:

```go
err := httpx.RunWithOptions(servingContext, server, httpx.RunOptions{
    ShutdownTimeout:     15 * time.Second,
    ForceCloseOnTimeout: true,
})
```

A zero timeout uses the five-second default. Forced closure preserves the
shutdown deadline error for `errors.Is(err, context.DeadlineExceeded)`.

The caller controls resource ordering:

1. Revoke readiness before canceling the serving context.
2. Drain HTTP while authentication stores and other request dependencies remain usable.
3. After the drain deadline, close remaining ordinary HTTP connections when opted in.
4. Complete application cleanup and flush telemetry within the total termination budget.

Separate listener contexts allow MCP traffic to drain before observability
stops. Do not use `RegisterOnShutdown` callbacks to enforce dependency ordering:
the standard library runs them asynchronously. There is no built-in delay for
endpoint propagation; determine whether one is needed from the deployment's
routing behavior rather than introducing an unconditional sleep.

As documented by [Go's HTTP server](https://pkg.go.dev/net/http#Server.Shutdown),
shutdown does not close hijacked connections. Forced ordinary connection closure
cancels request contexts, but cannot terminate handlers ignoring cancellation or
application jobs. SDK-specific stream cleanup remains the caller's responsibility.
The total [Kubernetes termination budget](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-termination-flow)
must accommodate termination hooks, HTTP draining, jobs and telemetry cleanup.
