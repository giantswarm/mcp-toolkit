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

// Icon returns svg as an MCP icon: an SVG data URI with sizes "any" and no
// theme. Use it for a server that shows another project's logo, typically
// embedded with go:embed. Keep the markup free of scripts and external
// references; muster drops what it does not allow.
func Icon(svg []byte) mcp.Icon {
	return mcp.Icon{
		Src:      "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(svg),
		MIMEType: mimeType,
		Sizes:    []string{"any"},
	}
}

// WithIcon returns a server option that sets [Icon] of svg as the server's
// icon.
func WithIcon(svg []byte) server.ServerOption {
	return server.WithIcons(Icon(svg))
}

// GiantSwarmIcon returns the Giant Swarm logo as an MCP icon, see [Icon].
func GiantSwarmIcon() mcp.Icon {
	return Icon(logo)
}

// WithGiantSwarmIcon returns a server option that sets [GiantSwarmIcon] as the
// server's icon.
func WithGiantSwarmIcon() server.ServerOption {
	return server.WithIcons(GiantSwarmIcon())
}
