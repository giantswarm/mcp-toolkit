package toolpolicy_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpgoserver "github.com/mark3labs/mcp-go/server"
	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	gopolicy "github.com/giantswarm/mcp-toolkit/middleware/gosdk"
	legacyPolicy "github.com/giantswarm/mcp-toolkit/middleware/mcpgo"
	"github.com/giantswarm/mcp-toolkit/toolpolicy"
)

type resultAdapter func(*testing.T, toolpolicy.Options, string) callResult

func resultAdapters() map[string]resultAdapter {
	return map[string]resultAdapter{
		"mcp-go": func(t *testing.T, options toolpolicy.Options, fixture string) callResult {
			t.Helper()
			var value mcpgo.CallToolResult
			require.NoError(t, json.Unmarshal([]byte(fixture), &value))
			server := mcpgoserver.NewMCPServer("contract", "1.0.0", legacyPolicy.New(options))
			server.AddTool(mcpgo.NewTool(contractTool), func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) { return &value, nil })
			response := server.HandleMessage(t.Context(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool"}}`))
			success, ok := response.(mcpgo.JSONRPCResponse)
			require.True(t, ok, "%#v", response)
			result := success.Result.(*mcpgo.CallToolResult)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			return callResult{toolError: result.IsError, message: string(encoded)}
		},
		"official Go SDK": func(t *testing.T, options toolpolicy.Options, fixture string) callResult {
			t.Helper()
			var value gosdk.CallToolResult
			require.NoError(t, json.Unmarshal([]byte(fixture), &value))
			result, err := gopolicy.New(options)(func(context.Context, *gosdk.CallToolRequest) (*gosdk.CallToolResult, error) { return &value, nil })(t.Context(), &gosdk.CallToolRequest{Params: &gosdk.CallToolParamsRaw{Name: contractTool}})
			require.NoError(t, err)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			return callResult{toolError: result.IsError, message: string(encoded)}
		},
	}
}

func TestCompleteResultLimits(t *testing.T) {
	large := strings.Repeat("x", 1024)
	fixtures := map[string]string{
		"text":          `{"content":[{"type":"text","text":"` + large + `"}]}`,
		"structured":    `{"content":[],"structuredContent":{"value":"` + large + `"}}`,
		"metadata":      `{"content":[],"_meta":{"value":"` + large + `"}}`,
		"resource":      `{"content":[{"type":"resource","resource":{"uri":"test://resource","mimeType":"text/plain","text":"` + large + `"}}]}`,
		"image":         `{"content":[{"type":"image","mimeType":"image/png","data":"` + base64.StdEncoding.EncodeToString([]byte(large)) + `"}]}`,
		"combined text": `{"content":[{"type":"text","text":"` + large[:120] + `"},{"type":"text","text":"` + large[:120] + `"}]}`,
	}
	for sdk, call := range resultAdapters() {
		t.Run(sdk, func(t *testing.T) {
			for name, fixture := range fixtures {
				t.Run(name, func(t *testing.T) {
					original := call(t, toolpolicy.Options{Timeout: -1}, fixture)
					for _, limit := range []int{-1, 0, len(original.message), len(original.message) - 1, 256} {
						reader := sdkmetric.NewManualReader()
						provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
						t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.WithoutCancel(t.Context()))) })
						var audit bytes.Buffer
						result := call(t, toolpolicy.Options{Timeout: -1, MaxResultBytes: limit, Meter: provider.Meter("limits"), Logger: slog.New(slog.NewJSONHandler(&audit, nil))}, fixture)
						limited := limit > 0 && len(original.message) > limit
						require.Equal(t, limited, result.toolError, "limit %d", limit)
						errorType := ""
						if limited {
							errorType = toolFailureOutcome
							require.Contains(t, result.message, "tool result exceeds")
						} else {
							require.JSONEq(t, original.message, result.message)
						}
						var event map[string]any
						require.NoError(t, json.Unmarshal(audit.Bytes(), &event))
						if limited {
							require.Equal(t, toolFailureOutcome, event["outcome"])
						} else {
							require.Equal(t, "success", event["outcome"])
						}
						assertDuration(t, reader, errorType)
					}
				})
			}
		})
	}
}
