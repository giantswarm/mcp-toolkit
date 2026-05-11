package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"

	"github.com/giantswarm/mcp-toolkit/logging"
)

// ExampleInit_basic shows the typical service composition root:
// install the logger, defer Shutdown, get OTLP delivery when the
// operator sets OTEL_EXPORTER_OTLP_LOGS_ENDPOINT or
// OTEL_LOGS_EXPORTER, otherwise fall back to JSON-on-stderr.
func ExampleInit_basic() {
	logger, shutdown, err := logging.Init(context.Background(),
		logging.WithLevel(slog.LevelInfo),
		logging.WithLoggerName("github.com/giantswarm/your-mcp"),
		logging.WithServiceName("your-mcp"),
		logging.WithServiceVersion("1.2.3"),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	slog.SetDefault(logger)
	slog.Info("ready")
}

// ExampleInit_otlpWithStderrMirror shows the "OTLP for Loki, stderr
// for kubectl logs" pattern: when OTLP is configured, records flow to
// the LoggerProvider; the extra JSON handler keeps a stderr stream
// alive so an operator running `kubectl logs your-pod` still sees
// log lines without round-tripping through the collector.
//
// The stderr handler is wrapped in WithTraceContextAttrs so the
// emitted records carry trace_id and span_id slog.Attrs — the OTLP
// primary attaches SpanContext to its native OTel LogRecord, but a
// plain JSONHandler does not read ctx for trace data. Wrapping it
// gives the stderr stream the same correlation surface Loki gets.
//
// In the non-OTLP fallback the primary handler is already writing to
// stderr; the extra handler doubles the stream, which is usually
// undesired in local dev. Gate the extra on a build flag or env var
// if both modes share a single bootstrap.
func ExampleInit_otlpWithStderrMirror() {
	stderrHandler := logging.WithTraceContextAttrs(
		slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	)
	logger, shutdown, err := logging.Init(context.Background(),
		logging.WithLevel(slog.LevelInfo),
		logging.WithLoggerName("github.com/giantswarm/your-mcp"),
		logging.WithServiceName("your-mcp"),
		logging.WithServiceVersion("1.2.3"),
		logging.WithExtraHandlers(stderrHandler),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	logger.Info("ready")
}

// ExampleInit_warningOnlyExtra shows that ExtraHandlers can filter
// independently of the primary. Here the primary handler (OTLP when
// configured, JSON-on-stderr otherwise) receives everything; the
// extra writes only Warn-and-above into an in-memory buffer that a
// supervising process might later inspect or ship.
func ExampleInit_warningOnlyExtra() {
	var buf bytes.Buffer
	warningHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})
	logger, shutdown, err := logging.Init(context.Background(),
		logging.WithLevel(slog.LevelInfo),
		logging.WithLoggerName("github.com/giantswarm/your-mcp"),
		logging.WithServiceName("your-mcp"),
		logging.WithServiceVersion("1.2.3"),
		logging.WithExtraHandlers(warningHandler),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	logger.Info("informational, goes to primary only")
	logger.Warn("warning, primary AND buf")
}

// ExampleInit_fileAuditLog shows a file handler kept alongside the
// primary handler, e.g. for an audit trail that must remain on local
// disk regardless of OTLP delivery status.
//
// The file's lifecycle is the caller's responsibility — the toolkit's
// Shutdown does not close ExtraHandlers' underlying io.Writers.
func ExampleInit_fileAuditLog() {
	auditFile, err := os.OpenFile("/var/log/your-mcp/audit.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	defer func() { _ = auditFile.Close() }()

	auditHandler := slog.NewJSONHandler(auditFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger, shutdown, err := logging.Init(context.Background(),
		logging.WithLevel(slog.LevelInfo),
		logging.WithLoggerName("github.com/giantswarm/your-mcp"),
		logging.WithServiceName("your-mcp"),
		logging.WithServiceVersion("1.2.3"),
		logging.WithExtraHandlers(auditHandler),
	)
	if err != nil {
		panic(err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	logger.Info("authenticated",
		slog.String("subject", "alice"),
		slog.String("action", "list_pods"),
	)
}
