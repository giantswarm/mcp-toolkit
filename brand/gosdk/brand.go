// Package gosdk provides the Giant Swarm icon in official Go SDK types.
package gosdk

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/giantswarm/mcp-toolkit/internal/brand"
)

// GiantSwarmIcon returns the embedded SVG as an MCP icon.
func GiantSwarmIcon() mcp.Icon {
	return mcp.Icon{Source: brand.GiantSwarmDataURI(), MIMEType: brand.MIMEType, Sizes: []string{"any"}}
}
