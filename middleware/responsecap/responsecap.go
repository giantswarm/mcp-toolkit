package responsecap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DefaultLimit fits typical structured tool responses while tripping
// pathologically broad queries before they flood the LLM context.
const DefaultLimit = 128 * 1024

// ErrCode is the value of the "error" field in the rejection payload.
const ErrCode = "response_too_large"

// Options configures New.
type Options struct {
	// Limit is the maximum byte size allowed for any TextContent in the
	// tool result.
	//
	// Zero (the zero value) applies DefaultLimit, so Options{} is safe by
	// default. Negative values disable capping entirely — an escape hatch
	// for tests and one-off debugging, not for production.
	Limit int

	// Exempt, if non-nil, returns true for tool names that should never
	// have their output capped. Use this for tools whose output is
	// bounded by nature (health checks, build info, …) and capping
	// would be pointless.
	Exempt func(toolName string) bool

	// AllowOverride, if non-nil, is consulted per request after Exempt.
	// Returning true lets that one call past the cap. Useful for tools
	// that expose an explicit "give me everything" argument; the caller
	// is the one overriding, the middleware is just permitting it.
	AllowOverride func(toolName string, req mcp.CallToolRequest) bool

	// Hint, if non-nil, returns per-tool guidance text included in the
	// rejection payload. nil falls back to a generic narrow-the-query
	// message.
	Hint func(toolName string) string
}

// Error is the JSON shape written into TextContent when a result is capped.
type Error struct {
	Error   string `json:"error"`
	Bytes   int    `json:"bytes"`
	Limit   int    `json:"limit"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// New returns a server.ToolHandlerMiddleware that replaces oversized
// TextContent in tool results with a typed response_too_large error and
// sets IsError. Non-text content is left untouched.
func New(opts Options) server.ToolHandlerMiddleware {
	hint := opts.Hint
	if hint == nil {
		hint = defaultHint
	}
	limit := opts.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			res, err := next(ctx, req)
			if err != nil || res == nil || limit < 0 {
				return res, err
			}
			name := req.Params.Name
			if opts.Exempt != nil && opts.Exempt(name) {
				return res, nil
			}
			if opts.AllowOverride != nil && opts.AllowOverride(name, req) {
				return res, nil
			}
			for i, c := range res.Content {
				t, ok := c.(mcp.TextContent)
				if !ok || len(t.Text) <= limit {
					continue
				}
				payload, _ := json.Marshal(Error{
					Error:   ErrCode,
					Bytes:   len(t.Text),
					Limit:   limit,
					Message: fmt.Sprintf("response is %d bytes, exceeds %d byte limit", len(t.Text), limit),
					Hint:    hint(name),
				})
				res.Content[i] = mcp.TextContent{Type: "text", Text: string(payload)}
				res.IsError = true
			}
			return res, nil
		}
	}
}

func defaultHint(string) string {
	return "narrow the query: tighten filters, reduce the time range, or request fewer items"
}
