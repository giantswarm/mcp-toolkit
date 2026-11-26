// Package toolcallmetadata lets an MCP server accept kagent's request-scoped
// tool-call metadata: values an agent's caller supplies for one request, such
// as the caller's own session id, that reach a tool in the call's `_meta`
// instead of the model's arguments.
//
// The contract is kagent's provisional, project-defined MCP extension
// [ExtensionID], not an official MCP extension. See docs/tool-call-metadata.md
// for its pinned specification and compatibility policy. A server declares
// the metadata keys it accepts in capabilities.extensions with [MustSettings]:
//
//	server.NewMCPServer("export", "1.0.0", server.WithExtensions(map[string]any{
//		toolcallmetadata.ExtensionID: toolcallmetadata.MustSettings("com.example/session"),
//	}))
//
// A client that implements the extension then sends, on tools/call only, the
// values of those keys as `_meta["dev.kagent/tool-call-metadata"]`, which
// [Values] and [String] read.
//
// The values are untrusted application data, not credentials. Servers must
// validate them and independently authenticate and authorize each request.
// HTTP MCP authorization uses audience-bound access tokens in the Authorization
// header; stdio servers should obtain credentials from the environment.
// Callers must not put tokens or other secrets in these values, and servers
// must not treat the values as credentials.
package toolcallmetadata

import (
	"fmt"
	"maps"
	"strings"
)

// ExtensionID is kagent's MCP extension for request-scoped tool-call
// metadata. Its provisional contract is documented in docs/tool-call-metadata.md.
const ExtensionID = "dev.kagent/tool-call-metadata"

// MustSettings returns the extension's settings object for a server that accepts
// keys: {"keys": [...]}. It panics on a key that is not a qualified metadata
// key (see [IsKey]), as a server declares its keys when it is built.
func MustSettings(keys ...string) map[string]any {
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
// its entry is not an object. The returned map is a shallow copy; nested values
// remain shared. Values does not check negotiation or the server's accepted keys.
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

// String returns the value of key in a request's `_meta` when it is a
// string.
func String(meta map[string]any, key string) (string, bool) {
	if !IsKey(key) {
		return "", false
	}
	entry, _ := meta[ExtensionID].(map[string]any)
	value, ok := entry[key].(string)
	return value, ok
}

// IsKey reports whether key satisfies kagent's qualified metadata key rule,
// such as com.example/session. The extension applies MCP's _meta key grammar
// with a mandatory prefix, a nonempty name, and no MCP-reserved namespace
// (second label modelcontextprotocol or mcp). Core MCP permits an optional
// prefix and empty name and does not constrain keys nested in extension objects.
func IsKey(key string) bool {
	prefix, name, ok := strings.Cut(key, "/")
	if !ok || !validName(name) {
		return false
	}
	index := 0
	for label := range strings.SplitSeq(prefix, ".") {
		if !validLabel(label) {
			return false
		}
		if index == 1 && (label == "modelcontextprotocol" || label == "mcp") {
			return false
		}
		index++
	}
	return true
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
