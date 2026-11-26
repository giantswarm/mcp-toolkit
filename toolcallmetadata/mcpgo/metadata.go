// Package mcpgo reads kagent's tool-call metadata from mcp-go requests.
package mcpgo

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/giantswarm/mcp-toolkit/toolcallmetadata"
)

// ValuesOf returns a shallow copy of the request's qualified metadata values,
// or nil when none are present. It does not check negotiation or accepted keys.
func ValuesOf(request mcp.CallToolRequest) map[string]any {
	if request.Params.Meta == nil {
		return nil
	}
	return toolcallmetadata.Values(request.Params.Meta.AdditionalFields)
}
