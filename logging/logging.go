package logging

import (
	"io"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/sdk/resource"
)

// Format selects the slog handler shape.
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

// Option configures Init. The zero set of options produces a logger
// writing to os.Stderr at slog.LevelInfo with auto-detected format.
type Option func(*config)

type config struct {
	format          Format
	level           slog.Level
	output          io.Writer
	loggerName      string
	serviceName     string
	serviceVersion  string
	extraHandlers   []slog.Handler
	resourceOptions []resource.Option
	stderrMirror    bool
}

// WithFormat overrides FormatAuto. Use FormatText to force text on a
// pod-deployed binary (rare), or FormatJSON to force JSON in local
// dev.
func WithFormat(f Format) Option {
	return func(c *config) { c.format = f }
}

// WithLevel sets the minimum slog level emitted by the non-OTLP
// handler. Defaults to slog.LevelInfo. OTLP mode is not affected —
// otelslog.Handler.Enabled defers to the LoggerProvider; configure
// OTLP-side filtering via the OTel SDK.
func WithLevel(level slog.Level) Option {
	return func(c *config) { c.level = level }
}

// WithOutput sets the io.Writer that the non-OTLP handler writes to.
// Nil and unset both mean os.Stderr.
func WithOutput(w io.Writer) Option {
	return func(c *config) { c.output = w }
}

// WithLoggerName identifies the instrumentation scope on emitted OTel
// LogRecords (OTel SDK's InstrumentationScope.Name). Conventionally
// the importing module path, e.g. "github.com/giantswarm/muster".
// Empty falls back to the LoggerProvider's default scope name, which
// is rarely useful as a Loki filter. OTLP mode only.
func WithLoggerName(name string) Option {
	return func(c *config) { c.loggerName = name }
}

// WithServiceName sets semconv.ServiceName on the OTLP LoggerProvider's
// Resource. OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES take
// precedence. OTLP mode only.
func WithServiceName(name string) Option {
	return func(c *config) { c.serviceName = name }
}

// WithServiceVersion sets semconv.ServiceVersion on the OTLP
// LoggerProvider's Resource. There is no OTEL_SERVICE_VERSION env
// var; pass the build version here or in OTEL_RESOURCE_ATTRIBUTES.
// OTLP mode only.
func WithServiceVersion(version string) Option {
	return func(c *config) { c.serviceVersion = version }
}

// WithExtraHandlers attaches additional slog.Handler sinks that
// receive every log record alongside the env-selected primary
// handler. Use them to mirror records to additional sinks — e.g. a
// JSON handler on os.Stderr kept on top of OTLP for `kubectl logs`
// debugging, a file handler for an audit trail, or a secondary OTel
// collector bridge. Each handler's Enabled is checked per record so
// extras can filter independently of the primary's level.
//
// ExtraHandlers' lifecycles are the caller's responsibility — the
// Shutdown returned by Init does not close them. Pass handlers over
// io.Writers that the caller owns (os.Stderr, an open *os.File)
// rather than ones that need closing.
//
// Stdlib slog handlers (JSONHandler, TextHandler) ignore ctx for
// trace data. Wrap an extra with WithTraceContextAttrs to inject
// trace_id / span_id from the active SpanContext on each record.
func WithExtraHandlers(handlers ...slog.Handler) Option {
	return func(c *config) { c.extraHandlers = append(c.extraHandlers, handlers...) }
}

// WithResourceOptions appends resource.Option values to the toolkit's
// resource.New option list. Use to add extra attributes
// (deployment.environment, custom labels) or detectors (k8s, AWS,
// GCP). The toolkit's own options — semconv ServiceName/Version,
// Process, OS, Container, FromEnv — are applied first; caller-supplied
// options follow. OTLP mode only.
func WithResourceOptions(opts ...resource.Option) Option {
	return func(c *config) { c.resourceOptions = append(c.resourceOptions, opts...) }
}

// WithStderrMirror, in OTLP mode, appends a slog.JSONHandler on
// os.Stderr (wrapped with WithTraceContextAttrs so records carry
// trace_id and span_id) to the ExtraHandlers list.
//
// Init returns an error when applied without OTLP logs configured
// (none of OTEL_EXPORTER_OTLP_LOGS_ENDPOINT,
// OTEL_EXPORTER_OTLP_ENDPOINT, or OTEL_LOGS_EXPORTER set): the
// non-OTLP primary already writes to os.Stderr, so a second sink
// would double-emit.
//
// Honours WithLevel.
func WithStderrMirror() Option {
	return func(c *config) { c.stderrMirror = true }
}

// baseHandler builds the text/JSON slog handler. Single source of
// FormatAuto detection in the package.
func baseHandler(c config) slog.Handler {
	out := c.output
	if out == nil {
		out = os.Stderr
	}
	format := c.format
	if format == FormatAuto {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			format = FormatJSON
		} else {
			format = FormatText
		}
	}
	hopts := &slog.HandlerOptions{Level: c.level}
	if format == FormatJSON {
		return slog.NewJSONHandler(out, hopts)
	}
	return slog.NewTextHandler(out, hopts)
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
