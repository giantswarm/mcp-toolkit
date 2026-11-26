# Request metadata

`requestmeta` reads application fields from SDK-independent MCP request `_meta`
maps. It defines no MCP extension, capability declaration, key allowlist or
application payload schema. `requestmeta/mcpgo.FromToolCall` extracts these
fields from an mcp-go tool-call request.

## Usage

```go
import (
    "github.com/mark3labs/mcp-go/mcp"

    metadatamcpgo "github.com/giantswarm/mcp-toolkit/requestmeta/mcpgo"
)

func sessionID(request mcp.CallToolRequest) (string, bool) {
    metadata := metadatamcpgo.FromToolCall(request)
    application, _ := metadata["io.giantswarm/tool-call-metadata"].(map[string]any)
    session, ok := application["session"].(string)
    return session, ok
}
```

The example's application key and `session` field are chosen by the application
and server. Reading that key neither implements nor negotiates an MCP extension.
Use a suitable vendor prefix for application metadata. MCP defines its
[top-level key syntax](https://modelcontextprotocol.io/specification/2026-07-28/basic#_meta);
keys inside application objects are not constrained by that syntax.

`requestmeta.Values` returns a shallow copy of an SDK-independent `_meta` map.
`mcpgo.FromToolCall` copies mcp-go's additional `_meta` fields; its progress token
remains on `request.Params.Meta.ProgressToken`. Nested objects and arrays remain shared. Missing metadata
returns nil. `requestmeta.String` reads a string directly from an SDK-independent map.
A runtime forwarding request metadata must supply it on every request that
needs it; these readers retain no state.

## Updating a server

Replace `github.com/giantswarm/mcp-toolkit/toolcallmetadata` imports with
`github.com/giantswarm/mcp-toolkit/requestmeta`, and update the SDK adapter
import to `github.com/giantswarm/mcp-toolkit/requestmeta/mcpgo`.
Replace `mcpgo.ValuesOf(request)` with `mcpgo.FromToolCall(request)`.

`ExtensionID`, `MustSettings` and `IsKey` are removed. Remove declarations of
`dev.kagent/tool-call-metadata` and its `keys` settings. `Values` and `FromToolCall`
return the metadata fields rather than an unwrapped kagent payload, so read
your application key explicitly as in the example above. The matching runtime
must forward that application key directly; a runtime sending the older nested
kagent payload needs updating at the same time.

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
