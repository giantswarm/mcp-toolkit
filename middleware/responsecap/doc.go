// Package responsecap caps oversized TextContent in mcp-go tool results
// with a typed response_too_large error and sets IsError.
//
// Truncation is intentionally not offered: a truncated-but-syntactically-
// valid prefix is the worst LLM failure mode because it produces silent
// wrong answers. Callers that genuinely need the full payload should
// expose a per-tool Bypass argument or split the tool into narrower
// variants whose results fit the limit.
package responsecap
