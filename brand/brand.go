package brand

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	brandasset "github.com/giantswarm/mcp-toolkit/internal/brand"
)

// Icon returns svg as an MCP icon: an SVG data URI with sizes "any" and no
// theme. Use it for a server that shows another project's logo, typically
// embedded with go:embed. Keep the markup free of scripts and external
// references; muster drops what it does not allow.
func Icon(svg []byte) mcp.Icon {
	return mcp.Icon{
		Src:      brandasset.DataURI(svg),
		MIMEType: brandasset.MIMEType,
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
	return mcp.Icon{Src: brandasset.GiantSwarmDataURI(), MIMEType: brandasset.MIMEType, Sizes: []string{"any"}}
}

// WithGiantSwarmIcon returns a server option that sets [GiantSwarmIcon] as the
// server's icon.
func WithGiantSwarmIcon() server.ServerOption {
	return server.WithIcons(GiantSwarmIcon())
}
