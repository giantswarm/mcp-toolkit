// Package logging provides slog-based logger construction and a
// network-address redaction helper for Giant Swarm MCP servers.
//
// New picks a structured handler — text for local dev, JSON when
// running inside a Kubernetes pod (auto-detected via
// KUBERNETES_SERVICE_HOST).
//
// RedactAddress scrubs IP addresses and URL userinfo before they land in
// logs. It is the only redaction primitive in this package because
// URL parsing plus IPv6 surgery is genuinely non-trivial; format-only
// redactions (e.g. hashing emails, masking tokens) are short enough
// that each server can implement its own opinion at the call site or
// in a custom slog.HandlerOptions.ReplaceAttr.
//
// The slog standard library already provides the LogValuer interface
// for type-driven redaction — see
// https://github.com/golang/go/blob/master/src/log/slog/example_logvaluer_secret_test.go
// — and external libraries such as github.com/m-mizutani/masq cover
// recursive struct walking. The toolkit deliberately does not pre-bake
// either pattern.
package logging
