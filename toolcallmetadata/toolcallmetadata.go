// Package toolcallmetadata reads application data from MCP tool-call _meta.
// It defines no extension identifier, capability settings or payload schema.
// Values are untrusted application data, not credentials.
package toolcallmetadata

import "maps"

// Values returns a shallow copy of a tool call's _meta. Nested values remain
// shared. It does not validate application data or negotiate extensions.
func Values(meta map[string]any) map[string]any {
	return maps.Clone(meta)
}

// String returns the value of key when it is a string.
func String(meta map[string]any, key string) (string, bool) {
	value, ok := meta[key].(string)
	return value, ok
}
