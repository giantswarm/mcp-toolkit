// Package health serves Kubernetes-style /healthz and /readyz HTTP
// endpoints for Giant Swarm MCP servers.
//
// # When to use this package
//
// MCP servers that don't otherwise depend on Kubernetes libraries
// (mcp-opsgenie, mcp-runbooks, search-mcp, …). It has no transitive
// dependencies beyond the standard library.
//
// MCP servers that already pull in sigs.k8s.io/controller-runtime —
// because they run controllers or informers (e.g. mcp-observability-
// platform, muster) — should prefer controller-runtime's own healthz
// primitives at sigs.k8s.io/controller-runtime/pkg/healthz. The cost
// is already paid, and they integrate with the manager's lifecycle.
//
// # Liveness and readiness semantics
//
// /healthz is unconditional 200: the only legitimate reason to fail
// liveness is a deadlock the kubelet should restart the pod over,
// which a static handler cannot detect anyway. Liveness must never
// flap.
//
// /readyz reads a single atomic ready flag toggled by the server.
// Servers call SetReady(true) once initialization is complete and
// SetReady(false) when graceful shutdown begins. Readiness is
// deliberately pod-local — probing shared downstreams from /readyz
// is a foot-gun: a transient hiccup in the downstream flips every
// replica's /readyz at once and the Service yanks its last endpoint.
package health
