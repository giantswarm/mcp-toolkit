// Package brand lets a Giant Swarm-built MCP server advertise a logo as its
// MCP icon, so that clients such as muster and the developer
// portal can show it next to the server.
//
// [WithGiantSwarmIcon] sets the icon in serverInfo.icons of the initialize
// result:
//
//	s := server.NewMCPServer("my-mcp", "1.0.0", brand.WithGiantSwarmIcon())
//
// A server that fronts one upstream project, such as Kubernetes or Prometheus,
// can show that project's logo instead with [WithIcon] and its own embedded
// SVG:
//
//	//go:embed prometheus.svg
//	var prometheusLogo []byte
//
//	s := server.NewMCPServer("mcp-prometheus", "1.0.0", brand.WithIcon(prometheusLogo))
//
// The logo is embedded and sent as a data URI, so it works without network
// access on either side. The full-colour mark reads on light and dark
// backgrounds, so the icon names no theme.
package brand
