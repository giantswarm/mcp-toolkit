package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/logging"
)

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := logging.New(logging.Options{Format: logging.FormatText, Output: &buf})
	l.Info("hello", "k", "v")
	out := buf.String()
	require.Contains(t, out, `msg=hello`)
	require.Contains(t, out, `k=v`)
	require.NotContains(t, out, `{`, "text output must not look like JSON")
}

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	l := logging.New(logging.Options{Format: logging.FormatJSON, Output: &buf})
	l.Info("hello", "k", "v")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, "hello", rec["msg"])
	require.Equal(t, "v", rec["k"])
	require.Equal(t, "INFO", rec["level"])
}

func TestNew_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := logging.New(logging.Options{
		Format: logging.FormatText,
		Level:  slog.LevelWarn,
		Output: &buf,
	})
	l.Info("muted")
	l.Warn("audible")
	out := buf.String()
	require.NotContains(t, out, "muted")
	require.Contains(t, out, "audible")
}

func TestNew_AutoPicksJSONInsideKubernetes(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	var buf bytes.Buffer
	l := logging.New(logging.Options{Output: &buf}) // Format zero == FormatAuto
	l.Info("hello")
	require.True(t, json.Valid(buf.Bytes()), "expected JSON output: %s", buf.String())
}

func TestNew_AutoPicksTextOutsideKubernetes(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	var buf bytes.Buffer
	l := logging.New(logging.Options{Output: &buf})
	l.Info("hello")
	require.Contains(t, buf.String(), "msg=hello")
	require.False(t, json.Valid(buf.Bytes()))
}

func TestNew_ZeroOptionsDoesNotPanic(t *testing.T) {
	// Output nil → os.Stderr, Format zero → FormatAuto, Level zero
	// → slog.LevelInfo. Capturing stderr would leak across tests, so
	// we just confirm the constructor accepts the zero value and
	// returns a usable logger.
	l := logging.New(logging.Options{})
	require.NotNil(t, l)
}

func TestHash(t *testing.T) {
	require.Equal(t, "", logging.Hash(""), "empty input must return empty")

	a := logging.Hash("alice@example.com")
	b := logging.Hash("alice@example.com")
	require.Equal(t, a, b, "same input must hash to same output")

	require.True(t, strings.HasPrefix(a, "redacted:"))
	require.Len(t, strings.TrimPrefix(a, "redacted:"), 16)

	require.NotEqual(t, a, logging.Hash("bob@example.com"))
}

func TestMaskToken(t *testing.T) {
	require.Equal(t, "", logging.MaskToken(""))
	require.Equal(t, "[token:5 chars]", logging.MaskToken("abcde"))
	// No token bytes appear in the output.
	out := logging.MaskToken("eyJhbGciOiJIUzI1NiJ9.payload.sig")
	require.NotContains(t, out, "eyJ")
	require.NotContains(t, out, "payload")
	require.NotContains(t, out, "sig")
}

func TestRedactURL(t *testing.T) {
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, logging.RedactURL(tc.in))
		})
	}
}

func TestRedactURL_InvalidURLFallsBackToIPRedaction(t *testing.T) {
	// A string that contains "://" but does not parse cleanly as a
	// URL. We don't care about preserving structure; we only care
	// that any IP it contains is still redacted.
	in := "garbage://[::not an ip:: 192.168.1.5"
	got := logging.RedactURL(in)
	require.Contains(t, got, "<redacted-ip>")
	require.NotContains(t, got, "192.168.1.5")
}

func TestEmail_LogValueRedactsViaSlog(t *testing.T) {
	var buf bytes.Buffer
	l := logging.New(logging.Options{Format: logging.FormatJSON, Output: &buf})
	l.Info("op", "email", logging.Email("alice@example.com"))

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.NotEqual(t, "alice@example.com", rec["email"])
	require.True(t, strings.HasPrefix(rec["email"].(string), "redacted:"),
		"email value should be redacted, got %v", rec["email"])
}

func TestToken_LogValueRedactsViaSlog(t *testing.T) {
	var buf bytes.Buffer
	l := logging.New(logging.Options{Format: logging.FormatJSON, Output: &buf})
	l.Info("op", "token", logging.Token("eyJhbGciOiJIUzI1NiJ9.payload.signature"))

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	tokenStr := rec["token"].(string)
	require.NotContains(t, tokenStr, "eyJ")
	require.NotContains(t, tokenStr, "payload")
	require.Regexp(t, `^\[token:\d+ chars\]$`, tokenStr)
}

func TestURL_LogValueRedactsViaSlog(t *testing.T) {
	var buf bytes.Buffer
	l := logging.New(logging.Options{Format: logging.FormatJSON, Output: &buf})
	l.Info("op", "url", logging.URL("https://alice:secret@10.0.0.1:6443/api"))

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	urlStr := rec["url"].(string)
	require.NotContains(t, urlStr, "alice")
	require.NotContains(t, urlStr, "secret")
	require.NotContains(t, urlStr, "10.0.0.1")
	require.Contains(t, urlStr, "<redacted-ip>")
	require.Contains(t, urlStr, "6443")
}
