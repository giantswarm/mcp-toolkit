package toolcallmetadata_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/toolcallmetadata"
)

const (
	sessionKey     = "com.example/session"
	settingsKeys   = "keys"
	emptyNameKey   = "com.example/"
	reservedKey    = "dev.mcp/x"
	unqualifiedKey = "plain"
)

func TestIsKey(t *testing.T) {
	for key, want := range map[string]bool{
		sessionKey:                   true,
		"example/session.v2":         true,
		"session":                    false,
		emptyNameKey:                 false,
		"io.modelcontextprotocol/ui": false,
		reservedKey:                  false,
		"com.example.mcp/x":          true,
		"progressToken":              false,
		"a/b":                        true,
		"com.ex-ample9/a-b_c.d":      true,
		"":                           false,
		"/session":                   false,
		"com..example/session":       false,
		"1example/session":           false,
		"example-/session":           false,
		"ex_ample/session":           false,
		"com.example/-session":       false,
		"com.example/session_":       false,
		"com.example/a/b":            false,
		"com.example/a b":            false,
		"com.example/é":              false,
		"com.mcp.tools/session":      false,
	} {
		require.Equal(t, want, toolcallmetadata.IsKey(key), key)
	}
}

func TestMustSettings(t *testing.T) {
	keys := []string{sessionKey}
	settings := toolcallmetadata.MustSettings(keys...)
	keys[0] = "com.example/other"
	require.Equal(t, map[string]any{settingsKeys: []any{sessionKey}}, settings)
	require.Equal(t, map[string]any{settingsKeys: []any{}}, toolcallmetadata.MustSettings(), "a server may accept no key")
	for _, key := range []string{"session", emptyNameKey, reservedKey} {
		require.Panics(t, func() { toolcallmetadata.MustSettings(sessionKey, key) }, key)
	}
}

func TestValues(t *testing.T) {
	meta := map[string]any{
		toolcallmetadata.ExtensionID: map[string]any{sessionKey: "a1", "com.example/n": 3.0, unqualifiedKey: "x"},
		sessionKey:                   "a top-level entry is not the extension's",
	}
	require.Equal(t, map[string]any{sessionKey: "a1", "com.example/n": 3.0}, toolcallmetadata.Values(meta))
	values := toolcallmetadata.Values(meta)
	values[sessionKey] = "changed"
	require.Equal(t, "a1", meta[toolcallmetadata.ExtensionID].(map[string]any)[sessionKey])
	require.Contains(t, meta[toolcallmetadata.ExtensionID], unqualifiedKey)
	for name, meta := range map[string]map[string]any{
		"no _meta":      nil,
		"no entry":      {sessionKey: "a1"},
		"not an object": {toolcallmetadata.ExtensionID: "a1"},
		"null entry":    {toolcallmetadata.ExtensionID: nil},
		"empty object":  {toolcallmetadata.ExtensionID: map[string]any{}},
		"invalid keys":  {toolcallmetadata.ExtensionID: map[string]any{unqualifiedKey: "x"}},
	} {
		require.Nil(t, toolcallmetadata.Values(meta), name)
	}
}

func TestString(t *testing.T) {
	for _, test := range []struct {
		name  string
		entry any
		key   string
		value string
		ok    bool
	}{
		{name: "string", entry: map[string]any{sessionKey: "a1"}, key: sessionKey, value: "a1", ok: true},
		{name: "empty string", entry: map[string]any{sessionKey: ""}, key: sessionKey, ok: true},
		{name: "number", entry: map[string]any{sessionKey: 3.0}, key: sessionKey},
		{name: "missing key", entry: map[string]any{}, key: sessionKey},
		{name: "null entry", key: sessionKey},
		{name: "not an object", entry: "a1", key: sessionKey},
		{name: "unqualified key", entry: map[string]any{unqualifiedKey: "x"}, key: unqualifiedKey},
		{name: "reserved key", entry: map[string]any{reservedKey: "x"}, key: reservedKey},
		{name: "empty name", entry: map[string]any{emptyNameKey: "x"}, key: emptyNameKey},
	} {
		t.Run(test.name, func(t *testing.T) {
			meta := map[string]any{toolcallmetadata.ExtensionID: test.entry}
			value, ok := toolcallmetadata.String(meta, test.key)
			require.Equal(t, test.value, value)
			require.Equal(t, test.ok, ok)
		})
	}
	value, ok := toolcallmetadata.String(nil, sessionKey)
	require.Empty(t, value)
	require.False(t, ok)
}
