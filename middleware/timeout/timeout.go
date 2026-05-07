package timeout

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DefaultTimeout caps a hung upstream from stalling the MCP pod
// while leaving headroom for slow legitimate calls (PromQL ranges,
// large kube list calls, …). 30s is the typical kubelet probe
// budget — short enough that a stuck call doesn't hold a worker
// indefinitely.
const DefaultTimeout = 30 * time.Second

// New returns middleware that wraps each tool call in
// context.WithTimeout(d).
//
// Zero d applies DefaultTimeout. Negative d disables the wrap;
// the parent context passes through unchanged. Use the negative
// form for tests or one-off debugging — not in production, where
// a tool call that hangs forever is a worker leak.
//
// When the toolkit-imposed deadline fires, the result is replaced
// with an IsError CallToolResult containing the timeout duration
// and tool name. A parent-context cancel propagates as-is so
// caller-driven cancellation is not misreported as a timeout.
func New(d time.Duration) server.ToolHandlerMiddleware {
	if d == 0 {
		d = DefaultTimeout
	}
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(parent context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if d < 0 {
				return next(parent, req)
			}
			ctx, cancel := context.WithTimeout(parent, d)
			defer cancel()

			res, err := next(ctx, req)

			// Replace with an IsError CallToolResult only when
			// our toolkit deadline fired AND the parent context
			// is still healthy. If the parent already errored
			// (cancel or its own deadline), let that propagate
			// — we don't masquerade caller-driven termination
			// as a toolkit-imposed timeout.
			if err != nil &&
				ctx.Err() == context.DeadlineExceeded &&
				parent.Err() == nil {
				return mcp.NewToolResultError(
					fmt.Sprintf("tool %q exceeded timeout of %s", req.Params.Name, d),
				), nil
			}
			return res, err
		}
	}
}
