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
	// User returns the user the call acts for, as a [slog.LogValuer] that
	// names them without profile data or credentials; mcp-oauth's
	// *providers.UserInfo is one. The audit record logs it under "user". A nil
	// result or an empty value leaves the user out of the record.
	User func(context.Context) slog.LogValuer
	// Logger receives tool and outcome records without argument values or errors.
	// Nil disables audit logging.
	Logger *slog.Logger
	// Meter defaults to the global toolkit meter. Identities are never metric labels.
	Meter metric.Meter
}
