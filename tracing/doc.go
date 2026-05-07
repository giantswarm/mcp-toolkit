// Package tracing installs an OpenTelemetry tracer provider configured
// from the standard OTEL_* environment variables. If no OTLP endpoint
// is set, Init returns a no-op shutdown but still installs the W3C
// TraceContext + Baggage propagator so inbound traceparent headers are
// honoured.
//
// The exporter is selected by go.opentelemetry.io/contrib/exporters/autoexport,
// which reads OTEL_TRACES_EXPORTER ("otlp" by default; "console" or
// "none" also supported) and OTEL_EXPORTER_OTLP_PROTOCOL
// ("http/protobuf" by default; "grpc" supported).
//
// Resource attributes come from process/OS/container detectors merged
// with OTEL_RESOURCE_ATTRIBUTES. Kubernetes attrs (k8s.pod.name,
// k8s.namespace.name, k8s.node.name) should be rendered into that
// variable via the downward API at deploy time, e.g.:
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
package tracing
