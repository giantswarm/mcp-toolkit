// Package metrics installs an OpenTelemetry meter provider configured
// from the standard OTEL_* environment variables.
//
// Init reads OTEL_METRICS_EXPORTER (or falls back to the OTLP endpoint
// envs) and picks the exporter via
// go.opentelemetry.io/contrib/exporters/autoexport — values:
//
//   - "otlp" (default when an OTLP endpoint is set) — push metrics to a
//     remote collector via OTLP/gRPC or OTLP/HTTP, selected by
//     OTEL_EXPORTER_OTLP_PROTOCOL.
//   - "prometheus" — self-host a /metrics endpoint on
//     OTEL_EXPORTER_PROMETHEUS_HOST:OTEL_EXPORTER_PROMETHEUS_PORT
//     (default localhost:9464). The autoexport-managed HTTP server is
//     drained by the returned Shutdown alongside the SDK.
//   - "console" — stdout, for local development.
//   - "none" — explicit no-op.
//
// Comma-separated values are accepted, e.g.
// OTEL_METRICS_EXPORTER=otlp,prometheus enables both pipelines from a
// single set of metric instruments — useful in environments that run
// both a remote OTel collector and a same-cluster Prometheus / Mimir.
//
// When no exporter is configured (no endpoint, no OTEL_METRICS_EXPORTER),
// Init returns a no-op Shutdown and leaves the global MeterProvider as
// the SDK's no-op default. The three OTel signals (traces, metrics,
// logs) are independent — Init does not require tracing or logging to
// be configured.
//
// Service identity is supplied two ways: the ServiceName and
// ServiceVersion fields on InitOptions are written as semconv
// attributes on the MeterProvider's Resource, and the standard
// OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES env vars override them.
// Kubernetes attributes (k8s.pod.name, k8s.namespace.name,
// k8s.node.name) should be rendered into OTEL_RESOURCE_ATTRIBUTES via
// the downward API at deploy time:
//
//	env:
//	  - name: POD_NAME
//	    valueFrom: { fieldRef: { fieldPath: metadata.name } }
//	  - name: POD_NAMESPACE
//	    valueFrom: { fieldRef: { fieldPath: metadata.namespace } }
//	  - name: NODE_NAME
//	    valueFrom: { fieldRef: { fieldPath: spec.nodeName } }
//	  - name: OTEL_RESOURCE_ATTRIBUTES
//	    value: "k8s.pod.name=$(POD_NAME),k8s.namespace.name=$(POD_NAMESPACE),k8s.node.name=$(NODE_NAME)"
//
// # Exemplars
//
// The OTel SDK's default exemplar filter (TraceBasedFilter) attaches
// the active span's TraceID to histogram observations recorded inside
// a span context. With this package wired alongside tracing.Init, the
// Prometheus exporter writes exemplars in the exposition format
// (Prometheus 2.26+, Mimir 2.6+) so Grafana's "click latency bucket →
// jump to trace" pivot works out of the box.
package metrics
