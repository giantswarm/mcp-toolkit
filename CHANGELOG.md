# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [Unreleased]

### Added

- `tracing` package: `tracing.Init` installs an OpenTelemetry tracer provider configured from the standard `OTEL_*` environment variables, with W3C TraceContext + Baggage propagation. Returns a no-op shutdown when neither an OTLP endpoint nor `OTEL_TRACES_EXPORTER` is set, but always installs the propagator so inbound `traceparent` headers continue to chain. Note: depends on `go.opentelemetry.io/contrib/exporters/autoexport`, which transitively pulls in the OTel exporter constellation (Prometheus bridge, stdoutlog, otlp metric/log exporters) — accepted in exchange for OTel-maintained protocol selection.

[Unreleased]: https://github.com/giantswarm/mcp-toolkit/tree/main
