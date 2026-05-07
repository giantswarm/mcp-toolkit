// Package timeout provides a tool-call timeout middleware for
// mark3labs/mcp-go servers.
//
// New returns a server.ToolHandlerMiddleware that wraps each tool
// call in context.WithTimeout. When the deadline fires, the result
// is replaced with an IsError CallToolResult containing actionable
// text — so the LLM sees "tool X exceeded timeout of Ys" rather
// than a silent hang or a generic context error.
//
// A parent-context cancel propagates unchanged: callers (or
// upstream cancellation flows) are not masqueraded as toolkit
// timeouts.
package timeout
