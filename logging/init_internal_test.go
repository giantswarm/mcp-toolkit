package logging

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// captureExporter records every Record it receives. The zero value is
// usable; Shutdown and ForceFlush are no-ops.
type captureExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *captureExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}
	return nil
}

func (e *captureExporter) Shutdown(context.Context) error   { return nil }
func (e *captureExporter) ForceFlush(context.Context) error { return nil }

func (e *captureExporter) collected() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]sdklog.Record, len(e.records))
	copy(out, e.records)
	return out
}

// restoreGlobalLoggerProvider snapshots the global OTel LoggerProvider
// and restores it on cleanup. initWithExporter mutates the global —
// without this, the first test that touches OTLP mode leaves a stub
// provider installed for every test that follows.
func restoreGlobalLoggerProvider(t *testing.T) {
	t.Helper()
	prev := global.GetLoggerProvider()
	t.Cleanup(func() { global.SetLoggerProvider(prev) })
}

// extraSink is a slog.Handler that records every record it sees as
// (level, message) tuples. Just enough to assert dispatch in tests.
type extraSink struct{ records []extraRecord }

type extraRecord struct {
	level slog.Level
	msg   string
}

func (s *extraSink) Enabled(context.Context, slog.Level) bool { return true }

func (s *extraSink) Handle(_ context.Context, r slog.Record) error {
	s.records = append(s.records, extraRecord{level: r.Level, msg: r.Message})
	return nil
}

func (s *extraSink) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s *extraSink) WithGroup(string) slog.Handler      { return s }

func TestInitWithExporter_ExtraHandlersReceiveRecords(t *testing.T) {
	restoreGlobalLoggerProvider(t)

	exp := &captureExporter{}
	extra := &extraSink{}
	logger, shutdown, err := initWithExporter(t.Context(), exp, config{
		loggerName:    "github.com/giantswarm/mcp-toolkit/logging/test",
		serviceName:   "muster",
		extraHandlers: []slog.Handler{extra},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	logger.Info("hello", "k", "v")
	require.NoError(t, shutdown(context.Background()))

	require.Len(t, exp.collected(), 1, "primary OTLP exporter must receive the record")
	require.Len(t, extra.records, 1, "ExtraHandlers must receive the record in OTLP mode")
	require.Equal(t, "hello", extra.records[0].msg)
	require.Equal(t, slog.LevelInfo, extra.records[0].level)
}

func TestInitWithExporter_ServiceIdentityOnResource_LoggerNameOnScope(t *testing.T) {
	restoreGlobalLoggerProvider(t)

	exp := &captureExporter{}
	logger, shutdown, err := initWithExporter(t.Context(), exp, config{
		loggerName:     "github.com/giantswarm/mcp-toolkit/logging/test",
		serviceName:    "muster",
		serviceVersion: "1.2.3-test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	logger.Info("hello", "k", "v")

	// Drain the BatchProcessor synchronously so we can assert on the
	// emitted record without sleeping or polling.
	require.NoError(t, shutdown(context.Background()))

	got := exp.collected()
	require.Len(t, got, 1, "expected exactly one captured record")
	r := got[0]

	require.Equal(t, "github.com/giantswarm/mcp-toolkit/logging/test", r.InstrumentationScope().Name)

	res := r.Resource()
	require.NotNil(t, res)
	name, ok := res.Set().Value(semconv.ServiceNameKey)
	require.True(t, ok, "service.name must be set on the Resource")
	require.Equal(t, "muster", name.AsString())
	version, ok := res.Set().Value(semconv.ServiceVersionKey)
	require.True(t, ok, "service.version must be set on the Resource")
	require.Equal(t, "1.2.3-test", version.AsString())
}

// TestInitWithExporter_WithFormatIgnoredInOTLPMode verifies that
// WithFormat is a non-OTLP option: the OTLP primary handler is always
// otelslog.Handler regardless of the requested slog format.
func TestInitWithExporter_WithFormatIgnoredInOTLPMode(t *testing.T) {
	restoreGlobalLoggerProvider(t)

	exp := &captureExporter{}
	logger, shutdown, err := initWithExporter(t.Context(), exp, config{
		// FormatJSON would force slog.NewJSONHandler in the non-OTLP
		// path; in OTLP mode it must be ignored.
		format:      FormatJSON,
		serviceName: "muster",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	logger.Info("hello")
	require.NoError(t, shutdown(context.Background()))

	// The record reached the OTLP capture exporter — confirming the
	// primary is the otelslog bridge, not the slog.JSONHandler that
	// FormatJSON would imply.
	require.Len(t, exp.collected(), 1)
}

// TestInitWithExporter_WithResourceOptions_AttachesCallerAttrs
// verifies that caller-supplied resource attributes land on the
// LoggerProvider's Resource alongside the toolkit defaults.
func TestInitWithExporter_WithResourceOptions_AttachesCallerAttrs(t *testing.T) {
	restoreGlobalLoggerProvider(t)

	exp := &captureExporter{}
	logger, shutdown, err := initWithExporter(t.Context(), exp, config{
		serviceName: "muster",
		resourceOptions: []resource.Option{resource.WithAttributes(
			attribute.String("deployment.environment", "production"),
			attribute.String("cluster.name", "glean"),
		)},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	logger.Info("hello")
	require.NoError(t, shutdown(context.Background()))

	got := exp.collected()
	require.Len(t, got, 1)
	res := got[0].Resource()
	require.NotNil(t, res)

	env, hasEnv := res.Set().Value(attribute.Key("deployment.environment"))
	require.True(t, hasEnv, "deployment.environment must be set on the Resource")
	require.Equal(t, "production", env.AsString())
	cluster, hasCluster := res.Set().Value(attribute.Key("cluster.name"))
	require.True(t, hasCluster, "cluster.name must be set on the Resource")
	require.Equal(t, "glean", cluster.AsString())

	// Sanity: toolkit defaults still applied.
	svcName, hasSvcName := res.Set().Value(semconv.ServiceNameKey)
	require.True(t, hasSvcName)
	require.Equal(t, "muster", svcName.AsString())
}
