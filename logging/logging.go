package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// Hash returns a stable, anonymized representation of s of the form
// "redacted:" + first 16 hex chars of SHA-256(s). Suitable for
// correlating log entries (e.g. all requests from the same user)
// without exposing the underlying value.
//
// Hash("") returns "".
func Hash(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return "redacted:" + hex.EncodeToString(sum[:8])
}

// MaskToken returns "[token:N chars]" without exposing any token
// bytes. Even prefixes can aid attacks on JWT-like formats, so the
// content is never logged — only the length, as a coarse sanity
// check.
//
// MaskToken("") returns "".
func MaskToken(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("[token:%d chars]", len(s))
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

// Email is a string type that redacts itself when logged via slog.
// Its LogValue returns Hash(string(e)).
//
// Use it when you control the type of the value being logged:
//
//	type Caller struct { Email logging.Email }
//	logger.Info("op", "email", caller.Email) // emits hash, not address
//
// Note: slog reflects into struct values, which bypasses LogValue on
// inner fields. Log Email as its own attribute, not as a field of a
// larger struct (see package doc).
type Email string

// LogValue implements slog.LogValuer.
func (e Email) LogValue() slog.Value { return slog.StringValue(Hash(string(e))) }

// Token is a string type that redacts itself when logged via slog.
// Its LogValue returns MaskToken(string(t)).
type Token string

// LogValue implements slog.LogValuer.
func (t Token) LogValue() slog.Value { return slog.StringValue(MaskToken(string(t))) }

// URL is a string type that redacts itself when logged via slog.
// Its LogValue returns RedactURL(string(u)).
//
// Note that this is a logging-side representation. For URL parsing
// or HTTP, continue to use net/url.URL.
type URL string

// LogValue implements slog.LogValuer.
func (u URL) LogValue() slog.Value { return slog.StringValue(RedactURL(string(u))) }
