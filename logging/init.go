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
// in OTLP mode and a hook for additional slog.Handler sinks. The
// non-OTLP path ignores LoggerName, ServiceName, and ServiceVersion —
// slog records carry the same identity through stream metadata (pod,
// container, namespace) added by the log collector. ExtraHandlers
// apply in both modes.
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
	// ExtraHandlers are slog.Handler instances that receive every log
	// record alongside the env-selected primary handler. Use them to
	// mirror records to additional sinks — e.g. a JSON handler on
	// os.Stderr kept on top of OTLP for `kubectl logs` debugging, a
	// file handler for an audit trail, or a secondary OTel collector
	// bridge. Each handler's Enabled is checked per record so extras
	// can filter independently of the primary's level.
	//
	// ExtraHandlers' lifecycles are the caller's responsibility — the
	// Shutdown returned by Init does not close them. Pass handlers
	// over io.Writers that the caller owns (os.Stderr, an open *os.File)
	// rather than ones that need closing.
	ExtraHandlers []slog.Handler
	// ResourceOptions are appended to the toolkit's resource.New
	// option list. Use to add extra attributes (deployment.environment,
	// custom labels) or extra detectors (k8s, AWS, GCP). The toolkit's
	// own options — semconv ServiceName/Version, Process, OS,
	// Container, FromEnv — are applied first. Only used in OTLP mode.
	ResourceOptions []resource.Option
	// BatchProcessorOptions are appended to the BatchProcessor used in
	// OTLP mode. Use to tune queue size, export interval, or export
	// timeout for high-throughput services. Empty keeps the SDK
	// defaults.
	BatchProcessorOptions []sdklog.BatchProcessorOption
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
// primary handler is an otelslog.Handler wired to an OTel
// LoggerProvider with a BatchProcessor and an autoexport-selected
// exporter. The provider is also registered as the global OTel
// LoggerProvider so any code that emits OTel logs directly (e.g. via
// otel/log/global.Logger) routes through it. Records carry the active
// span's TraceID and SpanID automatically (the OTel SDK pulls
// SpanContext from the call's context.Context).
//
// Otherwise the primary handler follows the same auto-detection as
// New: JSON inside a Kubernetes pod (KUBERNETES_SERVICE_HOST set),
// text otherwise.
//
// InitOptions.ExtraHandlers, when non-empty, fan out alongside the
// primary handler — every record reaches the primary plus each extra.
// This is how you keep stderr alive in OTLP mode (pass a
// slog.NewJSONHandler(os.Stderr, nil) as an extra), tee to a file,
// or mirror to a secondary backend. Both modes honour ExtraHandlers.
//
// Field applicability:
//
//   - Output applies only to the non-OTLP primary handler. OTLP routes
//     records to the LoggerProvider, not to a writer. Pass an extra
//     handler if you want OTLP and Output simultaneously.
//   - Level applies only to the non-OTLP primary handler.
//     otelslog.Handler.Enabled defers to the OTel LoggerProvider, not
//     to slog's Level filter; configure OTLP-side filtering via the
//     OTel SDK.
//   - LoggerName, ServiceName, ServiceVersion apply only to OTLP mode.
//   - ExtraHandlers apply in both modes.
//
// The OTLP path requires neither traces nor metrics to be configured —
// the three signals are independent.
func Init(ctx context.Context, opts InitOptions) (handler slog.Handler, shutdown Shutdown, err error) {
	if !otlpLogsConfigured() {
		return compose(baseHandler(opts.Options), opts.ExtraHandlers), noopShutdown, nil
	}

	exp, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("otlp log exporter: %w", err)
	}
	return initWithExporter(ctx, exp, opts)
}

// compose combines a primary handler with any caller-supplied extras
// into a single slog.Handler. Avoids the fan-out indirection when
// there are no extras.
func compose(primary slog.Handler, extras []slog.Handler) slog.Handler {
	if len(extras) == 0 {
		return primary
	}
	all := make([]slog.Handler, 0, 1+len(extras))
	all = append(all, primary)
	all = append(all, extras...)
	return newFanout(all)
}

// initWithExporter constructs the OTLP-mode handler against an
// explicit Exporter. The seam exists so the exporter is a parameter
// rather than a hidden side effect of autoexport reading the
// environment.
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

	res, err := buildResource(ctx, opts)
	if err != nil {
		return nil, nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp, opts.BatchProcessorOptions...)),
		sdklog.WithResource(res),
	)
	exporterOwned = true
	global.SetLoggerProvider(lp)
	primary := otelslog.NewHandler(opts.LoggerName, otelslog.WithLoggerProvider(lp))
	return compose(primary, opts.ExtraHandlers), lp.Shutdown, nil
}

// buildResource composes the SDK Resource for the LoggerProvider.
// Toolkit defaults run first; caller-supplied ResourceOptions follow.
func buildResource(ctx context.Context, opts InitOptions) (*resource.Resource, error) {
	var attrs []attribute.KeyValue
	if opts.ServiceName != "" {
		attrs = append(attrs, semconv.ServiceName(opts.ServiceName))
	}
	if opts.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(opts.ServiceVersion))
	}
	resourceOpts := []resource.Option{
		resource.WithAttributes(attrs...),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithFromEnv(),
	}
	resourceOpts = append(resourceOpts, opts.ResourceOptions...)
	res, err := resource.New(ctx, resourceOpts...)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("otel resource: %w", err)
	}
	return res, nil
}

// baseHandler builds the text/JSON slog handler. Single source of
// FormatAuto detection in the package.
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
// env vars opts in.
func otlpLogsConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_LOGS_EXPORTER") != ""
}
