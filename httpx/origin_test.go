package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/httpx"
)

func TestOriginProtection(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			for _, test := range []struct {
				origin, fetchSite string
				status            int
			}{
				{"", "", http.StatusNoContent},
				{"http://service.example.test", "", http.StatusNoContent},
				{"https://attacker.example.test", "", http.StatusForbidden},
				{"https://attacker.example.test", "same-origin", http.StatusForbidden},
				{"", "cross-site", http.StatusForbidden},
				{"null", "", http.StatusForbidden},
				{"http://service.example.test/path", "", http.StatusForbidden},
				{"http://user@service.example.test", "", http.StatusForbidden},
				{"http://service.example.test?", "", http.StatusForbidden},
			} {
				var calls atomic.Int32
				protected := httpx.ProtectOrigin(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					calls.Add(1)
					require.Equal(t, method, request.Method)
					require.Equal(t, test.fetchSite, request.Header.Get("Sec-Fetch-Site"))
					writer.WriteHeader(http.StatusNoContent)
				}))
				request := httptest.NewRequest(method, "http://service.example.test/mcp", nil)
				request.Header.Set("Origin", test.origin)
				request.Header.Set("Sec-Fetch-Site", test.fetchSite)
				response := httptest.NewRecorder()
				protected.ServeHTTP(response, request)
				require.Equal(t, test.status, response.Code, "origin %s", test.origin)
				require.Equal(t, test.status == http.StatusNoContent, calls.Load() == 1)
			}
		})
	}
}

func TestOriginDuplicateHeaders(t *testing.T) {
	protected := httpx.ProtectOrigin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("duplicate Origin reached handler") }))
	request := httptest.NewRequest(http.MethodPost, "http://service.example.test/mcp", nil)
	request.Header.Add("Origin", "http://service.example.test")
	request.Header.Add("Origin", "http://attacker.example.test")
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
}

func TestOriginLoopbackHost(t *testing.T) {
	protected := httpx.ProtectOrigin(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }))
	server := httptest.NewServer(protected)
	defer server.Close()
	for _, host := range []string{"localhost", "127.0.0.1", "[::1]", "attacker.example.test"} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		request.Host = host
		response, err := server.Client().Do(request)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		expected := http.StatusNoContent
		if host == "attacker.example.test" {
			expected = http.StatusForbidden
		}
		require.Equal(t, expected, response.StatusCode, host)
	}
}
