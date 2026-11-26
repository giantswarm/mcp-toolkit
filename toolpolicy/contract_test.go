package toolpolicy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpgoserver "github.com/mark3labs/mcp-go/server"
	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	gopolicy "github.com/giantswarm/mcp-toolkit/middleware/gosdk"
	legacyPolicy "github.com/giantswarm/mcp-toolkit/middleware/mcpgo"
	"github.com/giantswarm/mcp-toolkit/toolpolicy"
)

type callResult struct {
	toolError bool
	err       error
	message   string
}
type adapter func(context.Context, toolpolicy.Options, func(context.Context) (bool, error)) callResult

const (
	contractTool           = "test_tool"
	contractSubject        = "person-subject"
	caseToolFailure        = "tool failure"
	caseProtocolFailure    = "protocol failure"
	caseCallerCancellation = "caller cancellation"
	caseCallerDeadline     = "caller deadline"
	caseLateSuccess        = "late success"
	caseDisabledTimeout    = "disabled timeout"
	caseDefaultTimeout     = "default timeout"
	casePanic              = "panic"
	caseTimeout            = "timeout"
	caseSuccess            = "success"
	contractActor          = "actor-subject"
	auditSubjectField      = "subject"
	toolFailureOutcome     = "tool_error"
	cancelledOutcome       = "cancelled"
)

func adapters() map[string]adapter {
	return map[string]adapter{
		"mcp-go": func(ctx context.Context, options toolpolicy.Options, next func(context.Context) (bool, error)) callResult {
			var request mcpgo.CallToolRequest
			request.Params.Name = contractTool
			request.Params.Arguments = map[string]any{"credential": "argument-secret"}
			server := mcpgoserver.NewMCPServer("contract", "1.0.0", legacyPolicy.New(options))
			server.AddTool(mcpgo.NewTool(request.Params.Name), func(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				failed, err := next(ctx)
				if err != nil {
					return nil, err
				}
				if failed {
					return mcpgo.NewToolResultError(caseToolFailure), nil
				}
				return mcpgo.NewToolResultText(caseSuccess), nil
			})
			response := server.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","arguments":{"credential":"argument-secret"}}}`))
			if failure, ok := response.(mcpgo.JSONRPCError); ok {
				return callResult{err: errors.New(failure.Error.Message)}
			}
			result := response.(mcpgo.JSONRPCResponse).Result.(*mcpgo.CallToolResult)
			text, _ := result.Content[0].(mcpgo.TextContent)
			return callResult{toolError: result.IsError, message: text.Text}
		},
		"official Go SDK": func(ctx context.Context, options toolpolicy.Options, next func(context.Context) (bool, error)) callResult {
			request := &gosdk.CallToolRequest{Params: &gosdk.CallToolParamsRaw{Name: contractTool, Arguments: json.RawMessage(`{"credential":"argument-secret"}`)}}
			result, err := gopolicy.New(options)(func(ctx context.Context, _ *gosdk.CallToolRequest) (*gosdk.CallToolResult, error) {
				failed, err := next(ctx)
				if err != nil {
					return nil, err
				}
				return &gosdk.CallToolResult{Content: []gosdk.Content{&gosdk.TextContent{Text: "result"}}, IsError: failed}, nil
			})(ctx, request)
			if result == nil {
				return callResult{err: err}
			}
			text, _ := result.Content[0].(*gosdk.TextContent)
			return callResult{toolError: result.IsError, err: err, message: text.Text}
		},
	}
}

func TestNativePolicyContracts(t *testing.T) {
	for name, call := range adapters() {
		t.Run(name, func(t *testing.T) {
			for _, test := range []string{caseSuccess, caseToolFailure, caseProtocolFailure, caseTimeout, caseLateSuccess, caseCallerCancellation, caseCallerDeadline, caseDisabledTimeout, caseDefaultTimeout, casePanic} {
				t.Run(test, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						reader := sdkmetric.NewManualReader()
						provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
						t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.WithoutCancel(t.Context()))) })
						var audit bytes.Buffer
						options := toolpolicy.Options{Timeout: time.Second, Meter: provider.Meter("contract"), Logger: slog.New(slog.NewJSONHandler(&audit, nil)), User: func(context.Context) slog.LogValuer {
							return testLogValuer{value: slog.GroupValue(
								slog.String("issuer", "https://issuer.example.test"), slog.String(auditSubjectField, contractSubject),
								slog.Group("actor", slog.String("issuer", "https://actors.example.test"), slog.String(auditSubjectField, contractActor)),
							)}
						}}
						parent, cancel := context.WithCancel(t.Context())
						defer cancel()
						if test == caseCallerCancellation {
							cancel()
						}
						if test == caseCallerDeadline {
							var stop context.CancelFunc
							parent, stop = context.WithTimeout(parent, time.Second)
							defer stop()
							options.Timeout = time.Minute
						}
						if test == caseDisabledTimeout {
							options.Timeout = -1
						}
						if test == caseDefaultTimeout {
							options.Timeout = 0
						}
						handlerError := errors.New("backend-error-secret")
						result := call(parent, options, func(ctx context.Context) (bool, error) {
							switch test {
							case caseToolFailure:
								return true, nil
							case caseProtocolFailure:
								return false, handlerError
							case caseTimeout, caseCallerCancellation, caseCallerDeadline, caseDefaultTimeout:
								<-ctx.Done()
								return false, ctx.Err()
							case caseLateSuccess:
								<-ctx.Done()
								return false, nil
							case caseDisabledTimeout:
								time.Sleep(time.Minute)
								return false, nil
							case casePanic:
								panic("panic-secret")
							default:
								return false, nil
							}
						})
						outcome, errorType := caseSuccess, ""
						switch test {
						case caseToolFailure:
							outcome, errorType = toolFailureOutcome, toolFailureOutcome
							require.True(t, result.toolError)
							require.NoError(t, result.err)
						case caseProtocolFailure:
							outcome, errorType = "protocol_error", "_OTHER"
							require.ErrorContains(t, result.err, handlerError.Error())
						case caseTimeout, caseLateSuccess, caseDefaultTimeout:
							outcome, errorType = caseTimeout, caseTimeout
							require.True(t, result.toolError)
							require.NoError(t, result.err)
							require.Contains(t, result.message, "exceeded timeout")
						case caseCallerCancellation:
							outcome, errorType = cancelledOutcome, cancelledOutcome
							require.ErrorContains(t, result.err, context.Canceled.Error())
						case caseCallerDeadline:
							outcome, errorType = cancelledOutcome, cancelledOutcome
							require.ErrorContains(t, result.err, context.DeadlineExceeded.Error())
						case casePanic:
							outcome, errorType = "protocol_error", "_OTHER"
							require.EqualError(t, result.err, "internal tool error")
						default:
							require.NoError(t, result.err)
							require.False(t, result.toolError)
						}
						var event map[string]any
						require.NoError(t, json.Unmarshal(audit.Bytes(), &event))
						require.Equal(t, outcome, event["outcome"])
						user, ok := event["user"].(map[string]any)
						require.True(t, ok, "the audit record names the user")
						require.Equal(t, contractSubject, user[auditSubjectField])
						require.Equal(t, map[string]any{"issuer": "https://actors.example.test", auditSubjectField: contractActor}, user["actor"])
						require.NotContains(t, audit.String(), "secret")
						assertDuration(t, reader, errorType)
					})
				})
			}
		})
	}
}

func assertDuration(t *testing.T, reader *sdkmetric.ManualReader, errorType string) {
	t.Helper()
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &collected))
	require.Len(t, collected.ScopeMetrics, 1)
	require.Len(t, collected.ScopeMetrics[0].Metrics, 1)
	measurement := collected.ScopeMetrics[0].Metrics[0]
	require.Equal(t, "mcp.server.operation.duration", measurement.Name)
	require.Equal(t, "s", measurement.Unit)
	histogram, ok := measurement.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, histogram.DataPoints, 1)
	point := histogram.DataPoints[0]
	require.Equal(t, uint64(1), point.Count)
	method, _ := point.Attributes.Value("mcp.method.name")
	require.Equal(t, "tools/call", method.AsString())
	tool, _ := point.Attributes.Value("gen_ai.tool.name")
	require.Equal(t, contractTool, tool.AsString())
	failure, _ := point.Attributes.Value("error.type")
	require.Equal(t, errorType, failure.AsString())
	for _, attribute := range point.Attributes.ToSlice() {
		require.NotContains(t, string(attribute.Key), auditSubjectField)
		require.NotContains(t, string(attribute.Key), "issuer")
	}
}

func TestAuditLeavesOutAnAbsentUser(t *testing.T) {
	for name, call := range adapters() {
		t.Run(name, func(t *testing.T) {
			for _, lookup := range []func(context.Context) slog.LogValuer{
				nil,
				func(context.Context) slog.LogValuer { return nil },
				func(context.Context) slog.LogValuer { return testLogValuer{value: slog.GroupValue()} },
			} {
				var audit bytes.Buffer
				result := call(t.Context(), toolpolicy.Options{Timeout: -1, Logger: slog.New(slog.NewJSONHandler(&audit, nil)), User: lookup}, func(context.Context) (bool, error) { return false, nil })
				require.NoError(t, result.err)
				var event map[string]any
				require.NoError(t, json.Unmarshal(audit.Bytes(), &event))
				require.NotContains(t, event, "user")
			}
		})
	}
}

func TestAuditUsesTheSuppliedUserValue(t *testing.T) {
	for name, call := range adapters() {
		t.Run(name, func(t *testing.T) {
			for _, test := range []struct {
				name  string
				value slog.Value
				want  any
			}{
				{name: "zero value is null"},
				{name: "scalar", value: slog.StringValue("subject-id"), want: "subject-id"},
				{name: "application fields", value: slog.GroupValue(slog.String("account", "account-id")), want: map[string]any{"account": "account-id"}},
			} {
				t.Run(test.name, func(t *testing.T) {
					var audit bytes.Buffer
					result := call(t.Context(), toolpolicy.Options{
						Timeout: -1, Logger: slog.New(slog.NewJSONHandler(&audit, nil)),
						User: func(context.Context) slog.LogValuer { return testLogValuer{value: test.value} },
					}, func(context.Context) (bool, error) { return false, nil })
					require.NoError(t, result.err)
					var event map[string]any
					require.NoError(t, json.Unmarshal(audit.Bytes(), &event))
					require.Contains(t, event, "user")
					require.Equal(t, test.want, event["user"])
				})
			}
		})
	}
}

type testLogValuer struct{ value slog.Value }

func (user testLogValuer) LogValue() slog.Value { return user.value }
