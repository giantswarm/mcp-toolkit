package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	mcptoolkitotel "github.com/giantswarm/mcp-toolkit/internal/otel"
	"github.com/giantswarm/mcp-toolkit/logging"
)

// clearOTLPEnv blanks every OTLP logs env var so Init exercises the
// non-OTLP fallback path.
func clearOTLPEnv(t *testing.T) {
	t.Helper()
	t.Setenv(mcptoolkitotel.EnvExporterOTLPLogsEndpoint, "")
	t.Setenv(mcptoolkitotel.EnvExporterOTLPEndpoint, "")
	t.Setenv(mcptoolkitotel.EnvLogsExporter, "")
}

// enableOTLPLogsNone selects the OTLP code path via the no-op
// autoexport exporter.
func enableOTLPLogsNone(t *testing.T) {
	t.Helper()
	t.Setenv(mcptoolkitotel.EnvLogsExporter, "none")
	t.Setenv(mcptoolkitotel.EnvExporterOTLPLogsEndpoint, "")
	t.Setenv(mcptoolkitotel.EnvExporterOTLPEndpoint, "")
}

func TestInit_TextFormat(t *testing.T) {
	clearOTLPEnv(t)
	var buf bytes.Buffer
	l, _, err := logging.Init(context.Background(),
		logging.WithFormat(logging.FormatText), logging.WithOutput(&buf))
	require.NoError(t, err)
	l.Info("hello", "k", "v")
	out := buf.String()
	require.Contains(t, out, `msg=hello`)
	require.Contains(t, out, `k=v`)
	require.NotContains(t, out, `{`, "text output must not look like JSON")
}

func TestInit_JSONFormat(t *testing.T) {
	clearOTLPEnv(t)
	var buf bytes.Buffer
	l, _, err := logging.Init(context.Background(),
		logging.WithFormat(logging.FormatJSON), logging.WithOutput(&buf))
	require.NoError(t, err)
	l.Info("hello", "k", "v")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, "hello", rec["msg"])
	require.Equal(t, "v", rec["k"])
	require.Equal(t, "INFO", rec["level"])
}

func TestInit_LevelFiltering(t *testing.T) {
	clearOTLPEnv(t)
	var buf bytes.Buffer
	l, _, err := logging.Init(context.Background(),
		logging.WithFormat(logging.FormatText),
		logging.WithLevel(slog.LevelWarn),
		logging.WithOutput(&buf),
	)
	require.NoError(t, err)
	l.Info("muted")
	l.Warn("audible")
	out := buf.String()
	require.NotContains(t, out, "muted")
	require.Contains(t, out, "audible")
}

func TestInit_AutoPicksJSONInsideKubernetes(t *testing.T) {
	clearOTLPEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	var buf bytes.Buffer
	l, _, err := logging.Init(context.Background(), logging.WithOutput(&buf))
	require.NoError(t, err)
	l.Info("hello")
	require.True(t, json.Valid(buf.Bytes()), "expected JSON output: %s", buf.String())
}

func TestInit_AutoPicksTextOutsideKubernetes(t *testing.T) {
	clearOTLPEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	var buf bytes.Buffer
	l, _, err := logging.Init(context.Background(), logging.WithOutput(&buf))
	require.NoError(t, err)
	l.Info("hello")
	require.Contains(t, buf.String(), "msg=hello")
	require.False(t, json.Valid(buf.Bytes()))
}

func TestInit_NoOptionsDoesNotPanic(t *testing.T) {
	clearOTLPEnv(t)
	// Output nil → os.Stderr, Format zero → FormatAuto, Level zero
	// → slog.LevelInfo. Capturing stderr would leak across tests, so
	// we just confirm Init accepts no options and returns a usable
	// logger.
	l, shutdown, err := logging.Init(context.Background())
	require.NoError(t, err)
	require.NotNil(t, l)
	require.NoError(t, shutdown(context.Background()))
}

func TestRedactHost(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"bare ipv4", "192.168.1.10", "<redacted-ip>"},
		{"bare ipv4 with port", "192.168.1.10:6443", "<redacted-ip>:6443"},
		{"bare ipv6", "2001:db8::1", "<redacted-ip>"},
		{"plain hostname preserved", "api.example.com", "api.example.com"},
		{"plain hostname with port preserved", "api.example.com:6443", "api.example.com:6443"},
		{"url with ipv4", "https://192.168.1.10:6443/api", "https://<redacted-ip>:6443/api"},
		{"url with bracketed ipv6", "https://[2001:db8::1]:6443/api", "https://<redacted-ip>:6443/api"},
		{"url with hostname preserved", "https://api.example.com:6443/api", "https://api.example.com:6443/api"},
		{"url with userinfo stripped", "https://alice:secret@api.example.com/x", "https://api.example.com/x"},
		{"url with userinfo and ipv4", "https://alice:s@10.0.0.1:6443/x", "https://<redacted-ip>:6443/x"},
		{"bare host with userinfo", "alice:secret@redis.example.com:6379", "redis.example.com:6379"},
		{"bare host with userinfo and ipv4", "alice:secret@10.0.0.1:6379", "<redacted-ip>:6379"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, logging.RedactHost(tc.in))
		})
	}
}

func TestRedactHost_InvalidURLFallsBackToIPRedaction(t *testing.T) {
	// A string that contains "://" but does not parse cleanly as a
	// URL. We don't care about preserving structure; we only care
	// that any IP it contains is still redacted.
	in := "garbage://[::not an ip:: 192.168.1.5"
	got := logging.RedactHost(in)
	require.Contains(t, got, "<redacted-ip>")
	require.NotContains(t, got, "192.168.1.5")
}
