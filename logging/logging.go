package logging

import (
	"crypto/sha256"
	"encoding/hex"
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

// New returns an *slog.Logger writing to w, or os.Stderr when w is nil.
//
// FormatAuto resolves to FormatJSON when KUBERNETES_SERVICE_HOST is
// set, FormatText otherwise. Pass FormatText or FormatJSON to override.
func New(format Format, level slog.Level, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}
	if format == FormatAuto {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			format = FormatJSON
		} else {
			format = FormatText
		}
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == FormatJSON {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}

// HashEmail returns a stable, anonymized representation of an email of
// the form "user:" + first 16 hex chars of SHA-256(email). It allows
// correlation across log entries without exposing the address.
//
// HashEmail("") returns "".
func HashEmail(email string) string {
	if email == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(email))
	return "user:" + hex.EncodeToString(sum[:8])
}

const redactedIP = "<redacted-ip>"

var (
	ipv4Regex = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	// ipv6Regex matches common IPv6 forms including the bracketed
	// notation used in URLs ([2001:db8::1]).
	ipv6Regex = regexp.MustCompile(`\[?([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}\]?`)
)

// RedactURL returns s with IPv4/IPv6 addresses replaced by a redaction
// marker and userinfo stripped from URLs. Plain hostnames, ports, and
// paths are preserved.
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
