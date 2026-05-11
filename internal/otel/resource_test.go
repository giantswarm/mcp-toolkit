package otel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	mcptoolkitotel "github.com/giantswarm/mcp-toolkit/internal/otel"
)

func TestBuild_EmptyServiceFieldsOmitSemconvAttrs(t *testing.T) {
	res, err := mcptoolkitotel.Build(context.Background(), "", "", nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	_, hasName := res.Set().Value(semconv.ServiceNameKey)
	require.False(t, hasName, "empty serviceName must not write service.name")
	_, hasVersion := res.Set().Value(semconv.ServiceVersionKey)
	require.False(t, hasVersion, "empty serviceVersion must not write service.version")
}

func TestBuild_ServiceNameAndVersionWritten(t *testing.T) {
	res, err := mcptoolkitotel.Build(context.Background(), "muster", "1.2.3", nil)
	require.NoError(t, err)

	name, hasName := res.Set().Value(semconv.ServiceNameKey)
	require.True(t, hasName)
	require.Equal(t, "muster", name.AsString())
	version, hasVersion := res.Set().Value(semconv.ServiceVersionKey)
	require.True(t, hasVersion)
	require.Equal(t, "1.2.3", version.AsString())
}

func TestBuild_CallerExtrasAppendAndOverride(t *testing.T) {
	// Caller-supplied attributes apply after toolkit defaults. With
	// resource.WithAttributes last-write-wins, an extra attribute can
	// override an earlier-set value with the same key.
	res, err := mcptoolkitotel.Build(context.Background(), "muster", "1.2.3", []resource.Option{
		resource.WithAttributes(
			attribute.String("deployment.environment", "production"),
			attribute.String("service.version", "1.2.3-override"),
		),
	})
	require.NoError(t, err)

	env, hasEnv := res.Set().Value(attribute.Key("deployment.environment"))
	require.True(t, hasEnv, "caller extras must land on the Resource")
	require.Equal(t, "production", env.AsString())
	version, hasVersion := res.Set().Value(semconv.ServiceVersionKey)
	require.True(t, hasVersion)
	require.Equal(t, "1.2.3-override", version.AsString(),
		"caller extras run after defaults — last write wins")
}

func TestBuild_TolerantOfPartialResourceError(t *testing.T) {
	// resource.WithFromEnv parses OTEL_RESOURCE_ATTRIBUTES; a malformed
	// value triggers ErrPartialResource. Build must swallow it and
	// return the partial result rather than failing the whole init.
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "missing-equals-sign-here")
	res, err := mcptoolkitotel.Build(context.Background(), "muster", "1.0.0", nil)
	require.NoError(t, err)
	require.NotNil(t, res)
	// service.name still landed despite the partial-resource error.
	name, hasName := res.Set().Value(semconv.ServiceNameKey)
	require.True(t, hasName)
	require.Equal(t, "muster", name.AsString())
}
