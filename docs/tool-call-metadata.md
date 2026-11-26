# Tool-call metadata

`toolcallmetadata` implements the server-side declaration and readers for
kagent's project-defined MCP extension `dev.kagent/tool-call-metadata`.
It is not an official MCP extension or an OAuth implementation.

## Contract and compatibility

The contract is provisional. This implementation follows the
[pinned kagent proposal](https://github.com/kagent-dev/kagent/blob/0aee75521661a9c96e2a4c83059ee76c55154cc1/docs/architecture/mcp-tool-call-metadata-extension.md),
tracked in [kagent-dev/kagent#3807](https://github.com/kagent-dev/kagent/issues/3807).
Deploy it only with clients implementing that contract. Upstream acceptance
and interoperability with unrelated clients are not implied.

The identifier, `keys` settings field, and qualified-key rules belong to this
kagent extension. An incompatible change to the published wire contract must
use a new extension identifier, as required by
[MCP extension versioning](https://modelcontextprotocol.io/seps/2133-extensions).

## Usage

```go
import (
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"

    "github.com/giantswarm/mcp-toolkit/toolcallmetadata"
    metadatamcpgo "github.com/giantswarm/mcp-toolkit/toolcallmetadata/mcpgo"
)

func newServer() *server.MCPServer {
    return server.NewMCPServer("export", "1.0.0", server.WithExtensions(map[string]any{
        toolcallmetadata.ExtensionID: toolcallmetadata.MustSettings("com.example/session"),
    }))
}

func sessionID(request mcp.CallToolRequest) (string, bool) {
    session, ok := metadatamcpgo.ValuesOf(request)["com.example/session"].(string)
    return session, ok
}
```

`MustSettings` panics on an invalid key and is intended for server construction
with known keys. Validate configuration-derived keys with `IsKey` first.
`Values` and `String` take an SDK-independent `_meta` map; `mcpgo.ValuesOf`
handles mcp-go requests, including requests without metadata. `Values` and
`ValuesOf` return shallow copies: nested JSON objects and arrays remain shared.

## Negotiation and delivery

The client declares `capabilities.extensions["dev.kagent/tool-call-metadata"] = {}`.
The server declares the same extension with `{"keys":["com.example/session"]}`.
MCP 2025-11-25 exchanges capabilities during `initialize`; MCP 2026-07-28 carries
client capabilities on each request and exposes server capabilities through
`server/discover`. See [MCP extension negotiation](https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning#extension-negotiation).

The sending runtime or relay intersects the current request's keys with each
destination server's accepted keys. Only `tools/call` carries the resulting
object under `_meta["dev.kagent/tool-call-metadata"]`; an empty intersection
adds nothing. A request without values carries none. These readers do not
verify negotiation or enforce the accepted-key intersection.

`IsKey` applies the extension's stricter qualified-key rule: MCP `_meta` key
syntax with a mandatory vendor prefix, a nonempty name and no MCP-reserved
namespace. Core MCP allows an optional prefix and empty name and does not
constrain keys nested inside extension objects. See [MCP metadata](https://modelcontextprotocol.io/specification/2026-07-28/basic#_meta).

## Trust and authorization

Values are untrusted application data. Validate their types and meaning, then
authorize each operation independently. Bind a supplied session reference to
the authenticated caller; possession of the reference grants no authority.
Capability declarations express support and accepted keys, not permission to
receive confidential data. A server can return a value in a tool result, so
keeping it out of arguments does not guarantee it stays outside the model.

Do not carry access tokens, refresh tokens, API keys or other secrets in these
values, and do not treat them as credentials. HTTP MCP authorization uses
access tokens in the `Authorization` header; servers must validate that tokens
were issued for their resources. Stdio servers should obtain credentials from
the environment. See [MCP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization).
