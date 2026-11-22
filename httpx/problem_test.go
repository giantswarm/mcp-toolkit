package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/httpx"
)

func TestProblemResponse(t *testing.T) {
	for _, name := range []string{"request too large", "invalid status", "unencodable extension"} {
		t.Run(name, func(t *testing.T) {
			problem := httpx.Problem{Status: http.StatusRequestEntityTooLarge, Detail: "request exceeds the supported size", Extensions: map[string]any{"code": "request_too_large", "status": 200, "detail": "must not override", "instance": "must not override"}}
			expectedStatus := http.StatusRequestEntityTooLarge
			if name == "invalid status" {
				problem.Status = http.StatusOK
				expectedStatus = http.StatusInternalServerError
			}
			if name == "unencodable extension" {
				problem.Extensions["unsupported"] = make(chan int)
				expectedStatus = http.StatusInternalServerError
			}
			api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { httpx.WriteProblem(writer, problem) }))
			t.Cleanup(api.Close)
			response, err := api.Client().Get(api.URL)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
			require.Equal(t, expectedStatus, response.StatusCode)
			require.Equal(t, "application/problem+json", response.Header.Get("Content-Type"))
			var body map[string]any
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.Equal(t, float64(expectedStatus), body["status"])
			require.Equal(t, "about:blank", body["type"])
			require.Equal(t, http.StatusText(expectedStatus), body["title"])
			require.NotContains(t, body, "instance")
			if expectedStatus == http.StatusRequestEntityTooLarge {
				require.Equal(t, problem.Detail, body["detail"])
				require.Equal(t, "request_too_large", body["code"])
			} else {
				require.NotContains(t, body, "detail")
				require.NotContains(t, body, "code")
			}
		})
	}
}
