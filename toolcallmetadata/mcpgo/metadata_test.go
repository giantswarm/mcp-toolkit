package mcpgo_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/toolcallmetadata"
	metadatamcpgo "github.com/giantswarm/mcp-toolkit/toolcallmetadata/mcpgo"
)

const sessionKey = "com.example/session"

func TestValuesOf(t *testing.T) {
	require.Nil(t, metadatamcpgo.ValuesOf(mcp.CallToolRequest{}))
	for _, entry := range []any{nil, "a1", map[string]any{}, map[string]any{"plain": "x"}} {
		request := mcp.CallToolRequest{Params: mcp.CallToolParams{Meta: &mcp.Meta{AdditionalFields: map[string]any{
			toolcallmetadata.ExtensionID: entry,
		}}}}
		require.Nil(t, metadatamcpgo.ValuesOf(request))
	}
	original := map[string]any{sessionKey: "a1", "plain": "x"}
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Meta: &mcp.Meta{AdditionalFields: map[string]any{
		toolcallmetadata.ExtensionID: original,
	}}}}
	values := metadatamcpgo.ValuesOf(request)
	require.Equal(t, map[string]any{sessionKey: "a1"}, values)
	values[sessionKey] = "changed"
	require.Equal(t, "a1", original[sessionKey])
}

func TestNegotiatedToolCallMetadata(t *testing.T) {
	for _, version := range []string{mcp.ProtocolVersion20251125, mcp.ProtocolVersion20260728} {
		t.Run(version, func(t *testing.T) {
			mcpServer := server.NewMCPServer("export", "test", server.WithToolCapabilities(true), server.WithExtensions(map[string]any{
				toolcallmetadata.ExtensionID: toolcallmetadata.MustSettings(sessionKey),
			}))
			mcpServer.AddTool(mcp.NewTool("export"), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var capabilities mcp.ClientCapabilities
				if protocol := server.RequestProtocolInfoFromContext(ctx); protocol != nil && protocol.Modern {
					if protocol.ClientCapabilities == nil {
						return nil, errors.New("missing client capabilities")
					}
					capabilities = *protocol.ClientCapabilities
				} else if session, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo); ok {
					capabilities = session.GetClientCapabilities()
				}
				if _, supported := capabilities.Extensions[toolcallmetadata.ExtensionID]; !supported {
					return nil, errors.New("client did not advertise tool-call metadata")
				}
				value, _ := metadatamcpgo.ValuesOf(request)[sessionKey].(string)
				return mcp.NewToolResultText(value), nil
			})
			httpServer := httptest.NewServer(server.NewStreamableHTTPServer(mcpServer))
			t.Cleanup(httpServer.Close)

			httpTransport, err := transport.NewStreamableHTTP(httpServer.URL)
			require.NoError(t, err)
			mcpClient := client.NewClient(httpTransport, client.WithProtocolVersion(version))
			t.Cleanup(func() { require.NoError(t, mcpClient.Close()) })
			require.NoError(t, mcpClient.Start(t.Context()))
			var initialization mcp.InitializeRequest
			initialization.Params.Capabilities.Extensions = map[string]any{toolcallmetadata.ExtensionID: map[string]any{}}
			_, err = mcpClient.Initialize(t.Context(), initialization)
			require.NoError(t, err)
			require.Equal(t, version, mcpClient.ProtocolVersion())
			require.Equal(t, map[string]any{"keys": []any{sessionKey}}, mcpClient.GetServerCapabilities().Extensions[toolcallmetadata.ExtensionID])
			for _, expected := range []string{"first", "", "second"} {
				request := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "export"}}
				if expected != "" {
					request.Params.Meta = &mcp.Meta{AdditionalFields: map[string]any{
						toolcallmetadata.ExtensionID: map[string]any{sessionKey: expected},
					}}
				}
				result, err := mcpClient.CallTool(t.Context(), request)
				require.NoError(t, err)
				require.False(t, result.IsError)
				require.Len(t, result.Content, 1)
				require.Equal(t, expected, result.Content[0].(mcp.TextContent).Text)
			}
		})
	}
}
