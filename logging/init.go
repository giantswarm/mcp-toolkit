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

// resourceOpt is a thin alias so the Option func signature in
// logging.go doesn't need to import resource. resource.Option is the
// payload either way.
type resourceOpt = resource.Option

// batchOpt is a thin alias so the Option func signature in logging.go
// doesn't need to import sdklog. sdklog.BatchProcessorOption is the
// payload either way.
type batchOpt = sdklog.BatchProcessorOption

// WithResourceOptions appends resource.Option values to the toolkit's
// resource.New option list. Use to add extra attributes
// (deployment.environment, custom labels) or detectors (k8s, AWS,
// GCP). The toolkit's own options — semconv ServiceName/Version,
// Process, OS, Container, FromEnv — are applied first; caller-supplied
// options follow. OTLP mode only.
func WithResourceOptions(opts ...resource.Option) Option {
	return func(c *config) { c.resourceOptions = append(c.resourceOptions, opts...) }
}

// WithBatchProcessorOptions appends sdklog.BatchProcessorOption values
// to the BatchProcessor used in OTLP mode. Use to tune queue size,
// export interval, or export timeout for high-throughput services.
// Empty keeps the SDK defaults.
func WithBatchProcessorOptions(opts ...sdklog.BatchProcessorOption) Option {
	return func(c *config) { c.batchProcessorOptions = append(c.batchProcessorOptions, opts...) }
}

// Init returns an *slog.Logger and a Shutdown that drains the
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
// Otherwise the primary handler is JSON inside a Kubernetes pod
// (KUBERNETES_SERVICE_HOST set), text otherwise, with no OTLP
// pipeline — the Shutdown is a no-op closure.
//
// WithExtraHandlers, when applied, fans out alongside the primary —
// every record reaches the primary plus each extra. This is how to
// keep stderr alive in OTLP mode (pass a
// slog.NewJSONHandler(os.Stderr, nil) as an extra), tee to a file, or
// mirror to a secondary backend. Both modes honour ExtraHandlers.
//
// Callers that need to wrap the underlying handler (a domain
// subsystem layer, attribute redaction, etc.) can call
// Logger.Handler() on the returned value.
//
// Option applicability:
//
//   - WithOutput and WithLevel apply only to the non-OTLP primary
//     handler. OTLP routes records to the LoggerProvider, not to a
//     writer. otelslog.Handler.Enabled defers to the OTel
//     LoggerProvider, not to slog's Level filter; configure OTLP-side
//     filtering via the OTel SDK.
//   - WithLoggerName, WithServiceName, WithServiceVersion,
//     WithResourceOptions, WithBatchProcessorOptions apply only to
//     OTLP mode.
//   - WithExtraHandlers and WithFormat apply in both modes.
//
// The OTLP path requires neither traces nor metrics to be configured —
// the three signals are independent.
func Init(ctx context.Context, opts ...Option) (*slog.Logger, Shutdown, error) {
	c := config{}
	for _, opt := range opts {
		opt(&c)
	}
	if !otlpLogsConfigured() {
		return slog.New(compose(baseHandler(c), c.extraHandlers)), noopShutdown, nil
	}

	exp, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("otlp log exporter: %w", err)
	}
	return initWithExporter(ctx, exp, c)
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

// initWithExporter constructs the OTLP-mode logger against an
// explicit Exporter. The seam exists so the exporter is a parameter
// rather than a hidden side effect of autoexport reading the
// environment.
func initWithExporter(ctx context.Context, exp sdklog.Exporter, c config) (*slog.Logger, Shutdown, error) {
	exporterOwned := false
	defer func() {
		if exporterOwned {
			return
		}
		_ = exp.Shutdown(ctx)
	}()

	res, err := buildResource(ctx, c)
	if err != nil {
		return nil, nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp, c.batchProcessorOptions...)),
		sdklog.WithResource(res),
	)
	exporterOwned = true
	global.SetLoggerProvider(lp)
	primary := otelslog.NewHandler(c.loggerName, otelslog.WithLoggerProvider(lp))
	return slog.New(compose(primary, c.extraHandlers)), lp.Shutdown, nil
}

// buildResource composes the SDK Resource for the LoggerProvider.
// Toolkit defaults run first; caller-supplied ResourceOptions follow.
func buildResource(ctx context.Context, c config) (*resource.Resource, error) {
	var attrs []attribute.KeyValue
	if c.serviceName != "" {
		attrs = append(attrs, semconv.ServiceName(c.serviceName))
	}
	if c.serviceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(c.serviceVersion))
	}
	resourceOpts := []resource.Option{
		resource.WithAttributes(attrs...),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithFromEnv(),
	}
	resourceOpts = append(resourceOpts, c.resourceOptions...)
	res, err := resource.New(ctx, resourceOpts...)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("otel resource: %w", err)
	}
	return res, nil
}

func noopShutdown(context.Context) error { return nil }

// otlpLogsConfigured returns true when any of the standard OTEL log
// env vars opts in.
func otlpLogsConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_LOGS_EXPORTER") != ""
}
