package logging

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
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
