// Package toolpolicy configures SDK-native tool auditing, duration and deadlines.
package toolpolicy

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// Options contains policy shared by the native SDK adapters.
type Options struct {
	// MaxResultBytes bounds the complete JSON tool result. Nonpositive disables it.
	MaxResultBytes int
	// Timeout defaults to 30 seconds when zero. Negative disables the deadline.
	Timeout time.Duration
	// User returns an already-validated identity's log representation.
	// The callback owns its fields and redaction; its value is logged under
	// "user" without further validation. A nil result or empty slog group is
	// omitted by the standard handlers. A zero slog.Value is logged as null.
	User func(context.Context) slog.LogValuer
	// Logger receives tool and outcome records without argument values or errors.
	// Nil disables audit logging.
	Logger *slog.Logger
	// Meter defaults to the global toolkit meter. Identities are never metric labels.
	Meter metric.Meter
}
