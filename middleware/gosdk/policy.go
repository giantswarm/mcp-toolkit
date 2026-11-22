// Package gosdk applies common tool policy to native official Go SDK handlers.
package gosdk

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	policy "github.com/giantswarm/mcp-toolkit/internal/toolpolicy"
	"github.com/giantswarm/mcp-toolkit/toolpolicy"
)

// New wraps registered tool handlers after the SDK's dispatch and validation.
// Negotiation, protocol errors and result types remain native to the SDK.
func New(options toolpolicy.Options) func(mcp.ToolHandler) mcp.ToolHandler {
	configured := policy.New(options)
	return func(next mcp.ToolHandler) mcp.ToolHandler {
		next = limitResults(options.MaxResultBytes)(next)
		return func(parent context.Context, request *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
			ctx, cancel := configured.Context(parent)
			defer cancel()
			started := time.Now()
			outcome := policy.ProtocolError
			defer func() {
				if recover() != nil {
					result, err, outcome = nil, policy.ErrInternal, policy.ProtocolError
				}
				configured.Record(ctx, request.Params.Name, started, outcome)
			}()
			result, err = next(ctx, request)
			if configured.TimedOut(parent, ctx) {
				result, err = toolFailure(configured.TimeoutMessage(request.Params.Name)), nil
			}
			outcome = configured.Outcome(parent, ctx, err, result != nil && result.IsError)
			return result, err
		}
	}
}

func toolFailure(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: message}}, IsError: true}
}
