// Package logging provides slog-based logger construction and PII
// redaction primitives for Giant Swarm MCP servers.
//
// New picks a structured handler — text for local dev, JSON when
// running inside a Kubernetes pod (auto-detected via
// KUBERNETES_SERVICE_HOST).
//
// # Redaction
//
// Two complementary patterns. Pick whichever fits the call site; they
// compose.
//
//  1. LogValuer types (preferred). Wrap a sensitive string in Email,
//     Token, or URL and slog calls LogValue transparently. Redaction
//     travels with the value:
//
//     logger.Info("op", "email", logging.Email(user.Email))
//
//  2. Function helpers (for plain strings from third-party libraries).
//     Apply Hash, MaskToken, or RedactURL at the call site:
//
//     logger.Info("op", "url", logging.RedactURL(endpoint))
//
//     Or wire them into a custom slog.HandlerOptions.ReplaceAttr as a
//     defense-in-depth net for keys you can't always control.
//
// # Caveat: LogValuer is bypassed for struct fields
//
// slog reflects into struct and map values, and reflection skips
// LogValue on inner fields. Log the sensitive value as its own
// attribute, not as a field of a parent struct:
//
//	// Good — Email.LogValue is called.
//	logger.Info("op", "email", user.Email)  // user.Email is logging.Email
//
//	// Bad — User struct is reflected; Email.LogValue is skipped.
//	logger.Info("op", "user", user)
//
// When entire structs must be logged, redact at the handler via
// slog.HandlerOptions.ReplaceAttr using the function helpers above,
// or reach for an external library such as github.com/m-mizutani/masq
// which walks nested struct fields. The toolkit deliberately does not
// embed that dependency.
package logging
