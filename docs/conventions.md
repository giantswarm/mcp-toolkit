# Conventions

Shared shapes that consumer MCP servers should follow even when no toolkit code enforces them. Lives here, not in the README, so it can grow without bloating the repo landing page.

## Paginated tool results

Tools that return paginated lists return JSON of the shape:

```json
{
  "items": [/* ... */],
  "nextCursor": "..."
}
```

- The cursor field is camelCase (`nextCursor`) to match `mcp.PaginatedResult.NextCursor` at the protocol layer. Paginated tool *results* and paginated *list methods* (`tools/list`, `resources/list`, …) share semantics.
- Use `mcp.Cursor` from `github.com/mark3labs/mcp-go/mcp` (an alias for `string`) as the cursor type in your Go structs.
- `nextCursor` is **omitted** (or empty string) when there are no more pages.

```go
import "github.com/mark3labs/mcp-go/mcp"

type ListPodsResult struct {
    Items      []Pod      `json:"items"`
    NextCursor mcp.Cursor `json:"nextCursor,omitempty"`
}
```

### Cursor contents are tool-private

Encode whatever resume state your upstream needs:

- Pass through an opaque continue token verbatim (Kubernetes `Continue`, GitHub `after`, Stripe `starting_after`).
- Base64-JSON a small struct for offset-style or composite state.
- Whatever else your upstream requires.

The LLM treats cursors as opaque — it passes them back unchanged. The toolkit deliberately does **not** ship a cursor codec because there is no single right encoding for every upstream.

### Limit is a separate tool argument

```json
{ "name": "list_pods", "arguments": { "namespace": "x", "limit": 50, "cursor": "..." } }
```

- Limit is a regular tool parameter (`limit int`), not part of the cursor.
- Same cursor + different limit is a valid request — useful when the LLM wants to ask for a smaller page on the next call.
- Industry-universal: GitHub, Stripe, AWS `MaxResults`, gRPC `page_size`, Kubernetes `Limit`, Google APIs all separate them.

### Why no `hasMore` / `total` fields

- `nextCursor != ""` already says "more available". A separate `hasMore` invites drift between the two fields.
- `total` is rarely available cheaply (usually requires a second round-trip), and the LLM can infer "many" from the presence of `nextCursor`.

If a specific tool genuinely needs them (e.g. UI rendering downstream), add them to that tool's response — but don't add them to the convention.
