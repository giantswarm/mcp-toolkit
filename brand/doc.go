// Package brand lets a Giant Swarm-built MCP server advertise the Giant Swarm
// logo as its MCP icon, so that clients such as muster and the developer
// portal can show it next to the server.
//
// [WithGiantSwarmIcon] sets the icon in serverInfo.icons of the initialize
// result:
//
//	s := server.NewMCPServer("my-mcp", "1.0.0", brand.WithGiantSwarmIcon())
//
// The logo is embedded and sent as a data URI, so it works without network
// access on either side. The full-colour mark reads on light and dark
// backgrounds, so the icon names no theme.
package brand
