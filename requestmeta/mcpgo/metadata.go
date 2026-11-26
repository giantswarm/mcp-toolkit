// Package mcpgo reads tool-call metadata from mcp-go requests.
package mcpgo

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-toolkit/requestmeta"
)

// FromToolCall returns a shallow copy of mcp-go's additional _meta fields, or nil
// when absent. ProgressToken remains on request.Params.Meta.ProgressToken.
// It does not validate application data or negotiate extensions.
func FromToolCall(request mcp.CallToolRequest) map[string]any {
	if request.Params.Meta == nil {
		return nil
	}
	return requestmeta.Values(request.Params.Meta.AdditionalFields)
}
