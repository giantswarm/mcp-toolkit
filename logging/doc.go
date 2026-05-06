// Package logging provides slog-based logger construction and PII
// redaction helpers for Giant Swarm MCP servers.
//
// New picks a structured handler — text for local dev, JSON when
// running inside a Kubernetes pod (auto-detected via
// KUBERNETES_SERVICE_HOST). The redaction helpers HashEmail and
// RedactURL sanitize PII before it lands in logs.
//
// Per-server attribute helpers (cluster, namespace, resource, …) are
// intentionally not part of this package; they belong with the server
// that defines the domain.
package logging
