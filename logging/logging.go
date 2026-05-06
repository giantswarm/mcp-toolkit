package logging

import (
	"io"
	"log/slog"
	"net/url"
	"os"
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

// New returns an *slog.Logger configured per opts.
func New(opts Options) *slog.Logger {
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
	var h slog.Handler
	if format == FormatJSON {
		h = slog.NewJSONHandler(out, hopts)
	} else {
		h = slog.NewTextHandler(out, hopts)
	}
	return slog.New(h)
}

const redactedIP = "<redacted-ip>"

var (
	ipv4Regex = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	// ipv6Regex matches common IPv6 forms including the bracketed
	// notation used in URLs ([2001:db8::1]).
	ipv6Regex = regexp.MustCompile(`\[?([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}\]?`)
)

// RedactURL returns s with IPv4/IPv6 addresses replaced by a
// redaction marker and userinfo stripped from URLs. Plain hostnames,
// ports, and paths are preserved.
//
// Accepts either a full URL ("https://192.168.1.10:6443/path") or a
// bare host ("192.168.1.10:6443", "api.example.com"). RedactURL("")
// returns "".
//
// Use this when logging an error from an upstream API client whose
// message may include the API server address — e.g. a Kubernetes API
// error that interpolates the API server URL.
func RedactURL(s string) string {
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
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
