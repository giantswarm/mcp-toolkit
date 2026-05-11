package logging

import (
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
)

// Format selects the slog handler used by New.
type Format int

const (
	// FormatAuto picks JSON when KUBERNETES_SERVICE_HOST is set
	// (i.e. running inside a pod), otherwise text. This is the
	// usual choice for code that runs both locally and in-cluster.
	FormatAuto Format = iota
	// FormatText forces slog.TextHandler.
	FormatText
	// FormatJSON forces slog.JSONHandler.
	FormatJSON
)

// Options configures New. The zero value is usable: it produces a
// logger writing to os.Stderr at slog.LevelInfo with auto-detected
// format.
type Options struct {
	// Format selects the handler. Zero value FormatAuto resolves to
	// JSON inside Kubernetes, text otherwise.
	Format Format
	// Level is the minimum slog level emitted. Zero value is
	// slog.LevelInfo.
	Level slog.Level
	// Output is where log records are written. Nil means os.Stderr.
	Output io.Writer
}

// New returns an *slog.Logger configured per opts. The handler is the
// text/JSON one selected by opts.Format — New is the right call for
// CLI tools, tests, and any code path that does not need OpenTelemetry
// logs.
//
// For services that emit logs via OTLP and own a LoggerProvider
// lifecycle, use Init: it returns the slog.Handler plus a Shutdown
// closure and delegates handler construction here in the non-OTLP
// path.
func New(opts Options) *slog.Logger {
	return slog.New(baseHandler(opts))
}

const redactedIP = "<redacted-ip>"

var (
	ipv4Regex = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	// ipv6Regex matches common IPv6 forms including the bracketed
	// notation used in URLs ([2001:db8::1]).
	ipv6Regex = regexp.MustCompile(`\[?([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}\]?`)
)

// RedactHost returns s with IPv4/IPv6 addresses replaced by a
// redaction marker and userinfo stripped from URLs. Plain hostnames,
// ports, and paths are preserved.
//
// Accepts either a full URL ("https://192.168.1.10:6443/path") or a
// bare host ("192.168.1.10:6443", "api.example.com"). RedactHost("")
// returns "".
//
// Use this when logging an error from an upstream API client whose
// message may include the API server address — e.g. a Kubernetes API
// error that interpolates the API server URL.
func RedactHost(s string) string {
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		// Bare host. If it carries userinfo (e.g. a Redis/Valkey
		// address written as user:pass@host:port), parse under a
		// synthetic scheme to strip it, then redact any IP.
		if strings.Contains(s, "@") {
			if u, err := url.Parse("scheme://" + s); err == nil {
				u.User = nil
				return redactIPs(u.Host + u.Path)
			}
		}
		return redactIPs(s)
	}
	u, err := url.Parse(s)
	if err != nil {
		return redactIPs(s)
	}
	hasIP := ipv4Regex.MatchString(u.Host) || ipv6Regex.MatchString(u.Host)
	hasUser := u.User != nil
	if !hasIP && !hasUser {
		return s
	}
	if hasUser {
		u.User = nil
	}
	if hasIP {
		u.Host = redactIPs(u.Host)
	}
	return u.String()
}

func redactIPs(s string) string {
	s = ipv4Regex.ReplaceAllString(s, redactedIP)
	return ipv6Regex.ReplaceAllString(s, redactedIP)
}
