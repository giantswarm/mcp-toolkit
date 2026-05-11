package otel_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/internal/otel"
)

func TestConfigured(t *testing.T) {
	cases := []struct {
		name   string
		signal string
		env    map[string]string
		want   bool
	}{
		{
			name:   "traces: no env, not configured",
			signal: "traces",
			env:    map[string]string{},
			want:   false,
		},
		{
			name:   "traces: signal-specific OTLP endpoint",
			signal: "traces",
			env:    map[string]string{otel.EnvExporterOTLPTracesEndpoint: "http://collector:4318"},
			want:   true,
		},
		{
			name:   "metrics: shared OTLP endpoint",
			signal: "metrics",
			env:    map[string]string{otel.EnvExporterOTLPEndpoint: "http://collector:4318"},
			want:   true,
		},
		{
			name:   "logs: explicit exporter",
			signal: "logs",
			env:    map[string]string{otel.EnvLogsExporter: "console"},
			want:   true,
		},
		{
			name:   "metrics: prometheus exporter selection counts",
			signal: "metrics",
			env:    map[string]string{otel.EnvMetricsExporter: "prometheus"},
			want:   true,
		},
		{
			name:   "traces: other-signal env does NOT leak",
			signal: "traces",
			env:    map[string]string{otel.EnvMetricsExporter: "otlp"},
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{
				otel.EnvExporterOTLPTracesEndpoint,
				otel.EnvExporterOTLPMetricsEndpoint,
				otel.EnvExporterOTLPLogsEndpoint,
				otel.EnvExporterOTLPEndpoint,
				otel.EnvTracesExporter,
				otel.EnvMetricsExporter,
				otel.EnvLogsExporter,
			} {
				t.Setenv(key, "")
			}
			for key, val := range tc.env {
				t.Setenv(key, val)
			}
			require.Equal(t, tc.want, otel.Configured(tc.signal))
		})
	}
}
