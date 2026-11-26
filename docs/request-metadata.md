# Request metadata

Read MCP request metadata through the SDK used by the server. Applications own
their metadata keys and value types. Reading metadata does not require a toolkit
helper, an MCP extension declaration or a capability setting.

## Usage

```go
import "github.com/mark3labs/mcp-go/mcp"

func sessionID(request mcp.CallToolRequest) (string, bool) {
    if request.Params.Meta == nil {
        return "", false
    }
    application, _ := request.Params.Meta.AdditionalFields["io.giantswarm/tool-call-metadata"].(map[string]any)
    session, ok := application["session"].(string)
    return session, ok
}
```

The example's application key and `session` field are chosen by the application
and server. Reading that key neither implements nor negotiates an MCP extension.
Use a suitable vendor prefix for application metadata. MCP defines its
[top-level key syntax](https://modelcontextprotocol.io/specification/2026-07-28/basic#_meta);
keys inside application objects are not constrained by that syntax.

With mcp-go, application metadata is in `request.Params.Meta.AdditionalFields`;
the progress token is in `request.Params.Meta.ProgressToken`. Check for absent
metadata before reading the map. Use `maps.Clone` from the standard library
when a separate top-level map is needed; nested objects and arrays remain
shared. A runtime forwarding request metadata must supply it on every request
that needs it.

## Updating a server

The `github.com/giantswarm/mcp-toolkit/requestmeta` and `requestmeta/mcpgo`
packages are removed in the v0.11 minor release. Remove those imports and
replace `FromToolCall` with nil-checked SDK access as shown above. Replace
`Values` with `maps.Clone` when copying is required, and `String` with a string
type assertion.

The earlier `toolcallmetadata` packages and their `ExtensionID`, `MustSettings`,
`IsKey` and `ValuesOf` helpers are also unavailable. Remove declarations of
`dev.kagent/tool-call-metadata` and its `keys` settings. Read application fields
directly from `_meta` rather than unwrapping a kagent carrier. The sending
runtime must use the same direct metadata format.

## Trust and authorization

Values are untrusted application data. Validate their types and meaning, then
authorize each operation independently. Bind a supplied session reference to
the authenticated caller; possession of the reference grants no authority.
A server can return a value in a tool result, so keeping it out of arguments
does not guarantee that it stays outside the model.

Do not carry access tokens, refresh tokens, API keys or other secrets in these
values, and do not treat them as credentials. HTTP MCP authorization uses
access tokens in the Authorization header; servers validate that tokens were
issued for their resources. Stdio credentials come from the environment.
See [MCP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization).
