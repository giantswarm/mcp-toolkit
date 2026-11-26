package toolcallmetadata_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/toolcallmetadata"
)

const (
	sessionKey   = "com.example/session"
	settingsKeys = "keys"
)

func TestIsKey(t *testing.T) {
	for key, want := range map[string]bool{
		sessionKey:                   true,
		"example/session.v2":         true,
		"session":                    false,
		"com.example/":               false,
		"io.modelcontextprotocol/ui": false,
		"dev.mcp/x":                  false,
		"com.example.mcp/x":          true,
		"progressToken":              false,
	} {
		require.Equal(t, want, toolcallmetadata.IsKey(key), key)
	}
}

func TestSettings(t *testing.T) {
	require.Equal(t, map[string]any{settingsKeys: []any{sessionKey}}, toolcallmetadata.Settings(sessionKey))
	require.Equal(t, map[string]any{settingsKeys: []any{}}, toolcallmetadata.Settings(), "a server may accept no key")
	require.Panics(t, func() { toolcallmetadata.Settings("session") })
}

func TestValues(t *testing.T) {
	meta := map[string]any{
		toolcallmetadata.ExtensionID: map[string]any{sessionKey: "a1", "com.example/n": 3.0, "plain": "x"},
		sessionKey:                   "a top-level entry is not the extension's",
	}
	require.Equal(t, map[string]any{sessionKey: "a1", "com.example/n": 3.0}, toolcallmetadata.Values(meta))
	session, ok := toolcallmetadata.String(meta, sessionKey)
	require.True(t, ok)
	require.Equal(t, "a1", session)
	_, ok = toolcallmetadata.String(meta, "com.example/n")
	require.False(t, ok, "not a string")
	for name, meta := range map[string]map[string]any{
		"no _meta":      nil,
		"no entry":      {sessionKey: "a1"},
		"not an object": {toolcallmetadata.ExtensionID: "a1"},
	} {
		require.Nil(t, toolcallmetadata.Values(meta), name)
	}
}

func TestAServerDeclaresAndReadsTheValues(t *testing.T) {
	var got string
	srv := server.NewMCPServer("export", "test", server.WithToolCapabilities(true), server.WithExtensions(map[string]any{
		toolcallmetadata.ExtensionID: toolcallmetadata.Settings(sessionKey),
	}))
	srv.AddTool(mcp.NewTool("export"), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		got, _ = toolcallmetadata.ValuesOf(req)[sessionKey].(string)
		return mcp.NewToolResultText("ok"), nil
	})
	ts := httptest.NewServer(server.NewStreamableHTTPServer(srv))
	t.Cleanup(ts.Close)

	httpTransport, err := transport.NewStreamableHTTP(ts.URL)
	require.NoError(t, err)
	c := client.NewClient(httpTransport)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.Start(t.Context()))
	_, err = c.Initialize(t.Context(), mcp.InitializeRequest{})
	require.NoError(t, err)

	require.Equal(t, map[string]any{settingsKeys: []any{sessionKey}}, c.GetServerCapabilities().Extensions[toolcallmetadata.ExtensionID])
	_, err = c.CallTool(t.Context(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "export", Meta: &mcp.Meta{AdditionalFields: map[string]any{
		toolcallmetadata.ExtensionID: map[string]any{sessionKey: "a1"},
	}}}})
	require.NoError(t, err)
	require.Equal(t, "a1", got)
}
