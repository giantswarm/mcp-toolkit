package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown drains the LoggerProvider on graceful exit. In non-OTLP
// modes it is a no-op closure; in OTLP mode it forwards to
// LoggerProvider.Shutdown, which is itself safe to call more than once
// and from multiple goroutines.
type Shutdown func(ctx context.Context) error

// InitOptions extends Options with identifiers attached to log records
// in OTLP mode. The non-OTLP path ignores LoggerName, ServiceName, and
// ServiceVersion — slog records carry the same identity through stream
// metadata (pod, container, namespace) added by the log collector.
type InitOptions struct {
	Options
	// LoggerName identifies the instrumentation scope on emitted OTel
	// LogRecords (OTel SDK's InstrumentationScope.Name). Conventionally
	// the importing module path, e.g. "github.com/giantswarm/muster".
	// Empty falls back to the LoggerProvider's default scope name,
	// which is rarely useful as a Loki filter.
	LoggerName string
	// ServiceName is written as semconv.ServiceName on the
	// LoggerProvider's Resource when OTLP mode is selected. Empty
	// string lets OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES override.
	ServiceName string
	// ServiceVersion is written as semconv.ServiceVersion. There is
	// no OTEL_SERVICE_VERSION env var; the build version belongs in
	// this field or in OTEL_RESOURCE_ATTRIBUTES.
	ServiceVersion string
}

// Init returns the slog.Handler to use and a Shutdown that drains the
// LoggerProvider on graceful exit.
//
// Init must be called at most once per process. A second call installs
// a new global LoggerProvider, leaving the first one's BatchProcessor
// goroutine running with no way to recover a reference for shutdown.
//
// When OTLP logs are configured (any of OTEL_EXPORTER_OTLP_LOGS_ENDPOINT,
// OTEL_EXPORTER_OTLP_ENDPOINT, or OTEL_LOGS_EXPORTER is set), the
// returned handler is an otelslog.Handler wired to an OTel
// LoggerProvider with a BatchProcessor and an autoexport-selected
// exporter. The provider is also registered as the global OTel
// LoggerProvider so any code that emits OTel logs directly (e.g. via
// otel/log/global.Logger) routes through it. Records carry the active
// span's TraceID and SpanID automatically (the OTel SDK pulls
// SpanContext from the call's context.Context).
//
// In OTLP mode all records flow exclusively through the LoggerProvider —
// nothing is written to Output. If a deployment needs both OTLP delivery
// and a stderr-scraped log stream, the caller can wrap the returned
// handler in a fan-out slog.Handler that tees to slog.NewJSONHandler /
// slog.NewTextHandler on Options.Output. The toolkit's stance is
// single-pipeline-per-signal; teeing is a deliberate consumer decision.
//
// Otherwise the handler follows the same auto-detection as New: JSON
// inside a Kubernetes pod (KUBERNETES_SERVICE_HOST set), text otherwise.
//
// Field applicability:
//
//   - Output applies only to the non-OTLP path. OTLP routes records to
//     the LoggerProvider, not to a writer.
//   - Level applies only to the non-OTLP path. otelslog.Handler.Enabled
//     defers to the OTel LoggerProvider, not to slog's Level filter;
//     configure OTLP-side filtering via the OTel SDK.
//   - LoggerName, ServiceName, ServiceVersion apply only to OTLP mode.
//
// The OTLP path requires neither traces nor metrics to be configured —
// the three signals are independent.
func Init(ctx context.Context, opts InitOptions) (handler slog.Handler, shutdown Shutdown, err error) {
	if !otlpLogsConfigured() {
		return baseHandler(opts.Options), noopShutdown, nil
	}

	exp, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("otlp log exporter: %w", err)
	}
	return initWithExporter(ctx, exp, opts)
}

// initWithExporter constructs the OTLP-mode handler against an
// explicit Exporter. The seam exists so the exporter is a parameter
// rather than a hidden side effect of autoexport reading the
// environment; the package-internal test suite uses it to inject a
// record-capturing Exporter.
func initWithExporter(ctx context.Context, exp sdklog.Exporter, opts InitOptions) (slog.Handler, Shutdown, error) {
	// Hand exp ownership to the LoggerProvider on success; on any
	// error before that handover we must shut it down ourselves or
	// leak its underlying transport (gRPC client, batch goroutine).
	exporterOwned := false
	defer func() {
		if exporterOwned {
			return
		}
		_ = exp.Shutdown(ctx)
	}()

	var attrs []attribute.KeyValue
	if opts.ServiceName != "" {
		attrs = append(attrs, semconv.ServiceName(opts.ServiceName))
	}
	if opts.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(opts.ServiceVersion))
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithFromEnv(),
	)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, nil, fmt.Errorf("otel resource: %w", err)
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		sdklog.WithResource(res),
	)
	exporterOwned = true
	global.SetLoggerProvider(lp)
	return otelslog.NewHandler(opts.LoggerName,
		otelslog.WithLoggerProvider(lp),
	), lp.Shutdown, nil
}

// baseHandler builds the text/JSON slog handler used by New and by
// Init's non-OTLP path. Centralises the FormatAuto detection so the
// two entry points cannot diverge.
func baseHandler(opts Options) slog.Handler {
	out := opts.Output
	if out == nil {
		out = os.Stderr
	}
	format := opts.Format
	if format == FormatAuto {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			format = FormatJSON
		} else {
			format = FormatText
		}
	}
	hopts := &slog.HandlerOptions{Level: opts.Level}
	if format == FormatJSON {
		return slog.NewJSONHandler(out, hopts)
	}
	return slog.NewTextHandler(out, hopts)
}

func noopShutdown(context.Context) error { return nil }

// otlpLogsConfigured returns true when any of the standard OTEL log
// env vars opts in. Mirrors tracing.tracingConfigured / the metric
// equivalent in consumers, so the three signals follow the same
// shape.
func otlpLogsConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_LOGS_EXPORTER") != ""
}
