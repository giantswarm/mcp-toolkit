// Package toolpolicy configures SDK-native tool auditing, duration and deadlines.
package toolpolicy

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// Identity contains validated issuer and subject pairs. It contains no profile
// information or credentials. Actor fields are either both empty or both set.
type Identity struct {
	Issuer       string
	Subject      string
	ActorIssuer  string
	ActorSubject string
}

// Options contains policy shared by the native SDK adapters.
type Options struct {
	// MaxResultBytes bounds the complete JSON tool result. Nonpositive disables it.
	MaxResultBytes int
	// Timeout defaults to 30 seconds when zero. Negative disables the deadline.
	Timeout time.Duration
	// User reads an already-validated identity from the request context.
	// Missing or incomplete identities are omitted from audit records.
	User func(context.Context) (Identity, bool)
	// Logger receives tool and outcome records without argument values or errors.
	// Nil disables audit logging.
	Logger *slog.Logger
	// Meter defaults to the global toolkit meter. Identities are never metric labels.
	Meter metric.Meter
}
