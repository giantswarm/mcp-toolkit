package toolpolicy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	gopolicy "github.com/giantswarm/mcp-toolkit/middleware/gosdk"
	"github.com/giantswarm/mcp-toolkit/toolpolicy"
)

func TestNativePolicyOverStatelessHTTP(t *testing.T) {
	for _, test := range []struct {
		name      string
		limit     int
		oversized bool
	}{
		{name: "native result", limit: 256},
		{name: "complete result limit", limit: 256, oversized: true},
		{name: "disabled limit", oversized: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			type observation struct {
				hasSession, hasExtra, hasDeadline bool
				arguments                         string
			}
			observedRequests := make(chan observation, 1)
			var audit bytes.Buffer
			options := toolpolicy.Options{
				Timeout:        -1,
				MaxResultBytes: test.limit,
				Logger:         slog.New(slog.NewJSONHandler(&audit, nil)),
				User: func(context.Context) slog.LogValuer {
					return testLogValuer{value: slog.GroupValue(slog.String("issuer", "https://issuer.example"), slog.String(auditSubjectField, contractSubject))}
				},
			}
			native := mcp.NewServer(&mcp.Implementation{Name: "policy-contract", Version: "test"}, nil)
			native.AddTool(&mcp.Tool{Name: contractTool, InputSchema: map[string]any{"type": "object"}}, gopolicy.New(options)(func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				_, hasDeadline := ctx.Deadline()
				observedRequests <- observation{hasSession: request.Session != nil, hasExtra: request.Extra != nil, hasDeadline: hasDeadline, arguments: string(request.Params.Arguments)}
				value := "native"
				if test.oversized {
					value = strings.Repeat("x", 1024)
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}, StructuredContent: map[string]any{"value": value}, Meta: mcp.Meta{"custom": "preserved"}}, nil
			}))
			transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return native }, &mcp.StreamableHTTPOptions{Stateless: true})
			host := httptest.NewServer(transport)
			t.Cleanup(host.Close)
			client := mcp.NewClient(&mcp.Implementation{Name: "wire-contract", Version: "test"}, nil)
			session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: host.URL, HTTPClient: host.Client(), DisableStandaloneSSE: true}, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, session.Close()) })
			listed, err := session.ListTools(t.Context(), nil)
			require.NoError(t, err)
			require.Len(t, listed.Tools, 1)
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: contractTool, Arguments: map[string]any{"credential": "argument-secret"}})
			require.NoError(t, err)
			observed := <-observedRequests
			require.True(t, observed.hasSession)
			require.True(t, observed.hasExtra)
			require.False(t, observed.hasDeadline)
			require.Contains(t, observed.arguments, "argument-secret")
			limited := test.oversized && test.limit > 0
			require.Equal(t, limited, result.IsError)
			if limited {
				require.Nil(t, result.StructuredContent)
				require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "tool result exceeds")
			} else {
				require.Equal(t, "preserved", result.Meta["custom"])
				value, ok := result.StructuredContent.(map[string]any)
				require.True(t, ok)
				expected := "native"
				if test.oversized {
					expected = strings.Repeat("x", 1024)
				}
				require.Equal(t, expected, value["value"])
				require.Equal(t, "ok", result.Content[0].(*mcp.TextContent).Text)
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal(audit.Bytes(), &record))
			outcome := caseSuccess
			if limited {
				outcome = toolFailureOutcome
			}
			require.Equal(t, outcome, record["outcome"])
			require.Equal(t, map[string]any{"issuer": "https://issuer.example", auditSubjectField: contractSubject}, record["user"])
			require.NotContains(t, audit.String(), "argument-secret")
		})
	}
}
