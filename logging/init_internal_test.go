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

// captureExporter is an sdklog.Exporter that records every Record it
// receives. The Shutdown / ForceFlush implementations are intentionally
// minimal — production-facing concerns (retry, deadline) belong on the
// real OTLP exporter, not here. The zero value is usable.
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

func TestInitWithExporter_ServiceIdentityOnResource_LoggerNameOnScope(t *testing.T) {
	restoreGlobalLoggerProvider(t)

	exp := &captureExporter{}
	handler, shutdown, err := initWithExporter(t.Context(), exp, InitOptions{
		LoggerName:     "github.com/giantswarm/mcp-toolkit/logging/test",
		ServiceName:    "muster",
		ServiceVersion: "1.2.3-test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	slog.New(handler).Info("hello", "k", "v")

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
