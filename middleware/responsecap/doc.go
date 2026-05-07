// Package responsecap caps oversized TextContent in mcp-go tool results
// with a typed response_too_large error and sets IsError.
//
// The cap applies to CallToolResult.Content TextContent entries only.
// CallToolResult.Meta is left untouched, and non-text content
// (ImageContent, EmbeddedResource, …) is also untouched: tools must not
// use either to smuggle oversized payloads around the cap.
//
// Tool handlers are expected to return a fresh CallToolResult on every
// call — the standard mcp-go convention. responsecap edits the result
// in place; sharing one CallToolResult across calls is unsupported.
//
// Truncation is intentionally not offered: a truncated-but-syntactically-
// valid prefix is the worst LLM failure mode because it produces silent
// wrong answers. Callers that genuinely need the full payload should
// expose a per-tool Bypass argument or split the tool into narrower
// variants whose results fit the limit.
package responsecap
