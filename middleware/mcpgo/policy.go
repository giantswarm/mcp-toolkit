// Package mcpgo applies common tool policy to native mcp-go handlers.
package mcpgo

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	policy "github.com/giantswarm/mcp-toolkit/internal/toolpolicy"
	"github.com/giantswarm/mcp-toolkit/toolpolicy"
)

// New configures registered tool handlers and native whole-result limits.
// Protocol dispatch and result types remain native to mcp-go.
func New(options toolpolicy.Options) server.ServerOption {
	return func(target *server.MCPServer) {
		server.WithToolResultSizeLimit(options.MaxResultBytes)(target)
		server.WithToolHandlerMiddleware(middleware(options))(target)
	}
}

func middleware(options toolpolicy.Options) server.ToolHandlerMiddleware {
	configured := policy.New(options)
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(parent context.Context, request mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
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
				result, err = mcp.NewToolResultError(configured.TimeoutMessage(request.Params.Name)), nil
			}
			outcome = configured.Outcome(parent, ctx, err, result != nil && (result.IsError || configured.ResultTooLarge(result)))
			return result, err
		}
	}
}
