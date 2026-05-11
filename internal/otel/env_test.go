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
			env:    map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://collector:4318"},
			want:   true,
		},
		{
			name:   "metrics: shared OTLP endpoint",
			signal: "metrics",
			env:    map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318"},
			want:   true,
		},
		{
			name:   "logs: explicit exporter",
			signal: "logs",
			env:    map[string]string{"OTEL_LOGS_EXPORTER": "console"},
			want:   true,
		},
		{
			name:   "metrics: prometheus exporter selection counts",
			signal: "metrics",
			env:    map[string]string{"OTEL_METRICS_EXPORTER": "prometheus"},
			want:   true,
		},
		{
			name:   "traces: other-signal env does NOT leak",
			signal: "traces",
			env:    map[string]string{"OTEL_METRICS_EXPORTER": "otlp"},
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{
				"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
				"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
				"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
				"OTEL_EXPORTER_OTLP_ENDPOINT",
				"OTEL_TRACES_EXPORTER",
				"OTEL_METRICS_EXPORTER",
				"OTEL_LOGS_EXPORTER",
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
