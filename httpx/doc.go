// Package httpx provides HTTP serving and bounded graceful draining using
// net/http. Run preserves active connections after a drain timeout;
// RunWithOptions can close them explicitly with ForceCloseOnTimeout.
//
// Callers own readiness, signal handling, application jobs and dependent
// resources. Revoke readiness before canceling the serving context, drain HTTP
// while dependencies remain usable, then close dependencies and flush telemetry.
// RegisterOnShutdown callbacks execute asynchronously and cannot enforce this
// ordering. Separate listeners can use separate contexts to control drain order.
//
// A drain deadline is part of the total process termination budget, which must
// also cover termination hooks, application cleanup and telemetry flushing.
// Closing ordinary HTTP connections cancels their request contexts but cannot
// stop handlers that ignore cancellation. Hijacked connections and SDK-specific
// streaming cleanup remain the caller's responsibility.
//
// Serving uses ListenAndServe. TLS and custom listeners require a separate
// serving implementation using the standard library.
package httpx
