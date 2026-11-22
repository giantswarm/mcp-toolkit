package httpx

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ProtectOrigin enforces same-origin MCP requests on every HTTP method while
// accepting clients without browser-origin headers. Origin takes precedence over
// Sec-Fetch-Site; the standard library supplies authority and fetch-site checks.
func ProtectOrigin(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !validLocalHost(request) || len(request.Header.Values("Origin")) > 1 {
			http.Error(writer, "invalid request authority", http.StatusForbidden)
			return
		}
		origin := request.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
				http.Error(writer, "invalid Origin", http.StatusForbidden)
				return
			}
		}
		probe := request.Clone(request.Context())
		probe.Method = http.MethodPost
		if origin != "" {
			probe.Header.Del("Sec-Fetch-Site")
		}
		if err := protection.Check(probe); err != nil {
			http.Error(writer, "origin is not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func validLocalHost(request *http.Request) bool {
	address, ok := request.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || address == nil {
		return true
	}
	host, _, err := net.SplitHostPort(address.String())
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return true
	}
	host = (&url.URL{Host: request.Host}).Hostname()
	return strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()
}
