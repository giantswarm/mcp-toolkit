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
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown drains the LoggerProvider on graceful exit. In non-OTLP
// modes it is a no-op closure; in OTLP mode it forwards to
// LoggerProvider.Shutdown, which is itself safe to call more than once
// and from multiple goroutines.
type Shutdown func(ctx context.Context) error

// InitOptions extends Options with the service identity attached to
// log records in OTLP mode. The non-OTLP path ignores ServiceName and
// ServiceVersion — slog records carry the same identity through stream
// metadata (pod, container, namespace) added by the log collector.
type InitOptions struct {
	Options
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
// When OTLP logs are configured (any of OTEL_EXPORTER_OTLP_LOGS_ENDPOINT,
// OTEL_EXPORTER_OTLP_ENDPOINT, or OTEL_LOGS_EXPORTER is set), the
// returned handler is an otelslog.Handler wired to an OTel
// LoggerProvider with a BatchProcessor and an autoexport-selected
// exporter. Records carry the active span's TraceID and SpanID
// automatically (the OTel SDK pulls SpanContext from the call's
// context.Context).
//
// Otherwise the handler follows the same auto-detection as New: JSON
// inside a Kubernetes pod (KUBERNETES_SERVICE_HOST set), text
// otherwise. Output and Level are honoured in both paths; only OTLP
// mode uses ServiceName / ServiceVersion.
//
// The OTLP path requires neither traces nor metrics to be configured
// — the three signals are independent.
func Init(ctx context.Context, opts InitOptions) (slog.Handler, Shutdown, error) {
	if !otlpLogsConfigured() {
		return baseHandler(opts.Options), noopShutdown, nil
	}

	exp, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("otlp log exporter: %w", err)
	}

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
	return otelslog.NewHandler(opts.ServiceName,
		otelslog.WithLoggerProvider(lp),
	), lp.Shutdown, nil
}

// baseHandler builds the text/JSON handler used in non-OTLP mode.
// Mirrors the shape New produces, so callers that only care about the
// handler (no Shutdown needed) can keep using New.
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
