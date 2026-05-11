// Package otel holds helpers shared across the toolkit's three OTel
// signal packages (tracing, metrics, logging) — code that would
// otherwise be duplicated three times. Internal-only: not part of the
// public API surface.
package otel

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Build composes a *resource.Resource from the toolkit defaults plus
// caller-supplied extras.
//
// Service identity (serviceName, serviceVersion) is written as
// semconv attributes only when non-empty. Standard env var overrides
// (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES) take precedence over
// these values when set.
//
// The toolkit defaults run first (so caller-supplied attributes can
// override them where the SDK respects last-write semantics): semconv
// ServiceName/Version, then resource.WithProcess, WithOS,
// WithContainer, WithFromEnv. Caller extras follow.
//
// A partial-resource error is treated as success — the SDK returns
// ErrPartialResource when one detector fails but others succeed, and
// the partial result is still usable. Any other error is returned as
// fmt.Errorf("otel resource: %w", err).
func Build(ctx context.Context, serviceName, serviceVersion string, extra []resource.Option) (*resource.Resource, error) {
	var attrs []attribute.KeyValue
	if serviceName != "" {
		attrs = append(attrs, semconv.ServiceName(serviceName))
	}
	if serviceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(serviceVersion))
	}
	opts := []resource.Option{
		resource.WithAttributes(attrs...),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithFromEnv(),
	}
	opts = append(opts, extra...)
	res, err := resource.New(ctx, opts...)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("otel resource: %w", err)
	}
	return res, nil
}
