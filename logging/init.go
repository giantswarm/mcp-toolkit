package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	mcptoolkitotel "github.com/giantswarm/mcp-toolkit/internal/otel"
)

// Shutdown drains the LoggerProvider on graceful exit. In non-OTLP
// modes it is a no-op closure; in OTLP mode it forwards to
// LoggerProvider.Shutdown, which is itself safe to call more than once
// and from multiple goroutines.
type Shutdown func(ctx context.Context) error

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
// SpanContext from the call's context.Context). WithStderrMirror, if
// applied, synthesises an additional slog.JSONHandler on os.Stderr
// and appends it to the ExtraHandlers list — see its godoc.
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
//   - WithFormat, WithOutput apply only to the non-OTLP primary
//     handler. OTLP routes records to the LoggerProvider, not to a
//     writer; the primary handler in OTLP mode is always
//     otelslog.Handler, regardless of WithFormat.
//   - WithLevel applies to the non-OTLP primary handler and to the
//     stderr handler added by WithStderrMirror (when present).
//     otelslog.Handler's Enabled defers to the OTel LoggerProvider,
//     not to slog's Level filter; configure OTLP-side filtering via
//     the OTel SDK.
//   - WithLoggerName, WithServiceName, WithServiceVersion,
//     WithResourceOptions apply only to OTLP mode.
//   - WithStderrMirror requires OTLP mode and returns an error
//     otherwise.
//   - WithExtraHandlers applies in both modes.
//
// The OTLP path requires neither traces nor metrics to be configured —
// the three signals are independent.
func Init(ctx context.Context, opts ...Option) (*slog.Logger, Shutdown, error) {
	c := config{}
	for _, opt := range opts {
		opt(&c)
	}
	otlpMode := mcptoolkitotel.Configured("logs")

	if c.stderrMirror {
		if !otlpMode {
			return nil, nil, fmt.Errorf("logging: WithStderrMirror requires OTLP logs to be configured (set one of OTEL_EXPORTER_OTLP_LOGS_ENDPOINT, OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_LOGS_EXPORTER)")
		}
		c.extraHandlers = append(c.extraHandlers, newStderrMirrorHandler(os.Stderr, c.level))
	}

	if !otlpMode {
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

	res, err := mcptoolkitotel.Build(ctx, c.serviceName, c.serviceVersion, c.resourceOptions)
	if err != nil {
		return nil, nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		sdklog.WithResource(res),
	)
	exporterOwned = true
	global.SetLoggerProvider(lp)
	primary := otelslog.NewHandler(c.loggerName, otelslog.WithLoggerProvider(lp))
	return slog.New(compose(primary, c.extraHandlers)), lp.Shutdown, nil
}

func noopShutdown(context.Context) error { return nil }
