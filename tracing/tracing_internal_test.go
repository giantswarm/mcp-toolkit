package tracing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// TestConfig_WithResourceOptions verifies that WithResourceOptions
// accumulates resource.Option values that the buildResource helper
// will pass to resource.New. The end-to-end "attribute lands on the
// Resource" assertion lives in metrics' internal test against a
// ManualReader (the only signal whose Resource is readable from a
// test without a real exporter); here we check the config layer.
func TestConfig_WithResourceOptions(t *testing.T) {
	extra := attribute.String("deployment.environment", "production")
	c := config{}
	WithResourceOptions(resource.WithAttributes(extra))(&c)
	require.Len(t, c.resourceOptions, 1)
	WithResourceOptions(resource.WithAttributes(attribute.String("cluster.name", "glean")))(&c)
	require.Len(t, c.resourceOptions, 2)
}

// TestConfig_ServiceNameAndVersion verifies the WithServiceName /
// WithServiceVersion option setters populate the config — buildResource
// in internal/otel handles the semconv translation, which is covered
// by metrics' internal test through Record capture.
func TestConfig_ServiceNameAndVersion(t *testing.T) {
	c := config{}
	WithServiceName("svc")(&c)
	WithServiceVersion("1.2.3")(&c)
	require.Equal(t, "svc", c.serviceName)
	require.Equal(t, "1.2.3", c.serviceVersion)

	// Sanity: the semconv keys exist (compile-time regression guard).
	_ = semconv.ServiceName
	_ = semconv.ServiceVersion
}
