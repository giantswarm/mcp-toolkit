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

## Tool descriptions

A model behind muster picks its tool from a **summary**: `filter_tools` and `list_tools` answer with each description's first line, cut to 120 runes. The full description arrives with `describe_tool`, only after the pick, and an MCP client without muster loads every description on every request. So each sentence costs tokens on every listing, and the summary line decides whether the tool gets chosen at all. Build descriptions with [`tooldesc.Describe`](../tooldesc) and hold them to a budget in a test with `tooldesc.CheckServer` (giantswarm/giantswarm#38592).

```go
mcp.NewTool("update_repository",
    tooldesc.Describe(
        "Change a declared repository's team-file entry. Pass the whole entry, not a patch: omitted fields are removed.",
        "Lifecycle: set_lifecycle. Owner: transfer_repository. The team reviews the pull request.",
    ),
    …
)
```

```go
func TestToolDescriptions(t *testing.T) {
    require.NoError(t, tooldesc.CheckServer(newServer(), tooldesc.DefaultBudget))
}
```

Keep the text in review, too: a generated `docs/tools.md` shows every description and input schema exactly as a model reads them, and a test keeps it current. `tooldesc.RenderServerDoc` writes the page, each description in a fenced text block so GitHub interprets none of it, and the schemas as the tool sends them; `tooldesctest.AssertServerDoc` compares it with the file and writes it under `TOOLS_DOC_UPDATE=1`.

```go
func TestToolsDocIsCurrent(t *testing.T) {
    tooldesctest.AssertServerDoc(t, newServer(), "# The tools\n\nGenerated from the registered tools.", "../../docs/tools.md")
}
```

### The summary line

It stands on its own, at most 120 runes on one line:

- **What the tool does**, verb first: "List pods in a namespace", not "This tool lists…".
- **The sibling to use instead**, when one is close: "For an existing repository: adopt_repository."
- Or **the one thing the person must do**: "Deleting wipes code and issues; the person must type the name."

No shared opening across tools (`WRITES (as you, …)`, `Read-only.`): it fills the summary of every tool with the same words, so the summaries stop telling the tools apart. `Check` catches an opening shared across the first 40 runes (`Budget.Prefix`, ignoring case and leading space); a short one like `Read-only.` is left to review, since a check for it would also flag tools that just start with the same verb. Say read, write and destructive with the annotations instead — `readOnlyHint`, `destructiveHint`, `idempotentHint` — and set all three on purpose: a tool that sets only `readOnlyHint: true` is reported `destructiveHint: true` by mcp-go's defaults.

### What the details keep

Ask of every sentence where it would otherwise live:

| The sentence… | Goes |
|---|---|
| helps pick this tool, or another one instead | in the summary |
| prevents a mistake a capable model would make before any answer could correct it — irreversible, or reaching a person or system first | in the details or the parameter's description, one short clause |
| would be said anyway by the tool's error, refusal or dry run, before harm | into that error or refusal |
| explains the fields of the answer | into the answer: self-explanatory fields, a `pendingReason`, a `warning` |
| is about how the service works inside | into the README or docs |
| repeats the schema (enums, `required`, defaults, bounds) | nowhere |

Examples of details worth keeping: "pass the whole entry, not a patch: omitted fields are removed"; "confirm is the name the person typed: ask them, never fill it in"; "changes live settings: dry-run and show the person first"; "call again while pending is set".

### When an agent gets a tool wrong

Fix the tool's answer or refusal first: it costs tokens only when it is relevant, the description costs them on every listing. Extend the description only for a mistake no answer can correct — the wrong tool picked, or an irreversible call.

When a description does gain a sentence for such a mistake, add a test that asserts the fact it states — a short phrase, not the whole sentence — with the mistake in the failure message:

```go
func TestListKeepsItsOrderingFacts(t *testing.T) {
    d := toolByName(t, "list").Description
    for fact, why := range map[string]string{
        "alphabetical prefix": "a limited list of any other type is not the newest or the worst",
        "exact values":        "nothing selects by age: filter is equality-only",
    } {
        assert.Contains(t, d, fact, "list's description lost a fact from #559: %s", why)
    }
}
```

The description can then be reworded freely, and the fact still cannot be shortened away unnoticed. Keep these tests to the sentences that fixed an observed mistake; most description text needs none.

### Before you shorten a description

Find out why each sentence is there before you cut it. Pull requests are squash-merged, so the commit carries only the title: `git log -S'<sentence>'` names the commit, its `(#N)` the pull request, and the pull request's description the reason. A sentence added to fix what agents did wrong is a guardrail: keep its fact, in fewer words, and give it a test as above if it has none. A sentence that came with a feature and only describes it can usually go, or move into the answer.
