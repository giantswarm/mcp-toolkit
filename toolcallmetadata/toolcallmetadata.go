// Package toolcallmetadata lets an MCP server accept kagent's request-scoped
// tool-call metadata: values an agent's caller supplies for one request, such
// as the caller's own session id, that reach a tool in the call's `_meta`
// instead of the model's arguments.
//
// The contract is kagent's MCP extension [ExtensionID] (kagent's
// docs/architecture/mcp-tool-call-metadata-extension.md). A server declares
// the metadata keys it accepts in capabilities.extensions, with [Settings]:
//
//	server.NewMCPServer("export", "1.0.0", server.WithExtensions(map[string]any{
//		toolcallmetadata.ExtensionID: toolcallmetadata.Settings("com.example/session"),
//	}))
//
// A client that implements the extension then sends, on tools/call only, the
// values of those keys as `_meta["dev.kagent/tool-call-metadata"]`, which
// [Values] and [String] read.
//
// The values are untrusted application data, not credentials. A server
// validates them and authorizes its request from the transport's
// Authorization header, never from a value: a caller must not put tokens in
// them and a server must not accept one from them.
package toolcallmetadata

import (
	"fmt"
	"maps"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// ExtensionID is kagent's MCP extension for request-scoped tool-call
// metadata.
const ExtensionID = "dev.kagent/tool-call-metadata"

// Settings is the extension's settings object for a server that accepts
// keys: {"keys": [...]}. It panics on a key that is not a qualified metadata
// key (see [IsKey]), as a server declares its keys when it is built.
func Settings(keys ...string) map[string]any {
	accepted := make([]any, 0, len(keys))
	for _, key := range keys {
		if !IsKey(key) {
			panic(fmt.Sprintf("toolcallmetadata: %q is not a qualified metadata key", key))
		}
		accepted = append(accepted, key)
	}
	return map[string]any{"keys": accepted}
}

// Values returns the tool-call metadata of a request's `_meta`: the values
// keyed by qualified metadata key, or nil when the request carries none or
// its entry is not an object.
func Values(meta map[string]any) map[string]any {
	entry, ok := meta[ExtensionID].(map[string]any)
	if !ok {
		return nil
	}
	values := maps.Clone(entry)
	maps.DeleteFunc(values, func(key string, _ any) bool { return !IsKey(key) })
	if len(values) == 0 {
		return nil
	}
	return values
}

// ValuesOf returns the tool-call metadata of an mcp-go tool call (see
// [Values]).
func ValuesOf(req mcp.CallToolRequest) map[string]any {
	if req.Params.Meta == nil {
		return nil
	}
	return Values(req.Params.Meta.AdditionalFields)
}

// String returns the value of key in a request's `_meta` when it is a
// string.
func String(meta map[string]any, key string) (string, bool) {
	value, ok := Values(meta)[key].(string)
	return value, ok
}

// IsKey reports whether key is a qualified metadata key: the `_meta` key
// grammar of MCP with a mandatory prefix that MCP does not reserve (second
// label modelcontextprotocol or mcp), such as com.example/session.
func IsKey(key string) bool {
	prefix, name, ok := strings.Cut(key, "/")
	if !ok || !validName(name) {
		return false
	}
	labels := strings.Split(prefix, ".")
	for _, label := range labels {
		if !validLabel(label) {
			return false
		}
	}
	return len(labels) < 2 || labels[1] != "modelcontextprotocol" && labels[1] != "mcp"
}

func validLabel(label string) bool {
	if label == "" || !isLetter(label[0]) {
		return false
	}
	if last := label[len(label)-1]; !isLetter(last) && !isDigit(last) {
		return false
	}
	for i := 1; i < len(label)-1; i++ {
		if c := label[i]; !isLetter(c) && !isDigit(c) && c != '-' {
			return false
		}
	}
	return true
}

func validName(name string) bool {
	if name == "" || !isAlnum(name[0]) || !isAlnum(name[len(name)-1]) {
		return false
	}
	for i := 1; i < len(name)-1; i++ {
		if c := name[i]; !isAlnum(c) && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool  { return isLetter(c) || isDigit(c) }
