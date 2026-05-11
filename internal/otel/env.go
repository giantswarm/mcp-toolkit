package otel

import (
	"os"
	"strings"
)

// Standard OTel env vars the toolkit reads.
const (
	EnvExporterOTLPEndpoint        = "OTEL_EXPORTER_OTLP_ENDPOINT"
	EnvExporterOTLPTracesEndpoint  = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	EnvExporterOTLPMetricsEndpoint = "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"
	EnvExporterOTLPLogsEndpoint    = "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"
	EnvTracesExporter              = "OTEL_TRACES_EXPORTER"
	EnvMetricsExporter             = "OTEL_METRICS_EXPORTER"
	EnvLogsExporter                = "OTEL_LOGS_EXPORTER"
)

// Configured reports whether the operator has opted into exporting
// the given OTel signal. signal is the lower-case OTel name —
// "traces", "metrics", or "logs" — corresponding to the standard env
// var grammar. The check is the union of three env vars:
//
//   - OTEL_EXPORTER_OTLP_<SIGNAL>_ENDPOINT (signal-specific OTLP endpoint)
//   - OTEL_EXPORTER_OTLP_ENDPOINT          (shared OTLP endpoint)
//   - OTEL_<SIGNAL>_EXPORTER               (explicit exporter selection;
//     e.g. "console", "none",
//     "prometheus" for metrics)
//
// Any non-empty value triggers Configured to return true; the toolkit
// uses this to decide whether to spin up an SDK pipeline or return a
// no-op Shutdown.
func Configured(signal string) bool {
	upper := strings.ToUpper(signal)
	return os.Getenv("OTEL_EXPORTER_OTLP_"+upper+"_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_"+upper+"_EXPORTER") != ""
}
