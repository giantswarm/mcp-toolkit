package brand

import (
	_ "embed"
	"encoding/base64"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// logo is the Giant Swarm full-colour mark, minified from
// giantswarm/web-assets assets/app-icons/giantswarm/1/light.svg.
//
//go:embed giantswarm.svg
var logo []byte

const mimeType = "image/svg+xml"

// GiantSwarmIcon returns the Giant Swarm logo as an MCP icon: an SVG data URI
// with sizes "any" and no theme.
func GiantSwarmIcon() mcp.Icon {
	return mcp.Icon{
		Src:      "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(logo),
		MIMEType: mimeType,
		Sizes:    []string{"any"},
	}
}

// WithGiantSwarmIcon returns a server option that sets [GiantSwarmIcon] as the
// server's icon.
func WithGiantSwarmIcon() server.ServerOption {
	return server.WithIcons(GiantSwarmIcon())
}
