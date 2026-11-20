// Package health provides Kubernetes-style liveness and readiness handlers.
//
// Liveness reports whether the HTTP handler can respond. It does not detect
// stalled workers or guarantee that requests can complete. Restart-triggering
// checks should identify failures that restarting the process can repair.
//
// Readiness starts false. Call SetReady(true) once initialization completes,
// and SetReady(false) before initiating HTTP shutdown. Dependencies used by
// in-flight requests must remain available until draining completes.
//
// Dependency checks depend on whether useful requests can still be served.
// Avoid withdrawing every replica for a shared dependency outage when the
// application can continue serving useful responses. Keep probes inexpensive
// and avoid disclosing dependency errors or credentials in their responses.
//
// Consumers already using controller-runtime's manager should prefer its
// healthz primitives and lifecycle integration rather than another manager.
package health
