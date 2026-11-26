package mcpgo_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	metadatamcpgo "github.com/giantswarm/mcp-toolkit/toolcallmetadata/mcpgo"
)

const applicationKey = "io.giantswarm/tool-call-metadata"

func TestToolCallMetadataReachesApplicationWithoutExtraCapability(t *testing.T) {
	for _, version := range []string{mcp.ProtocolVersion20251125, mcp.ProtocolVersion20260728} {
		t.Run(version, func(t *testing.T) {
			observed := make(chan map[string]any, 1)
			mcpServer := server.NewMCPServer("export", "test", server.WithToolCapabilities(true))
			mcpServer.AddTool(mcp.NewTool("export"), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				observed <- metadatamcpgo.ValuesOf(request)
				return mcp.NewToolResultText("ok"), nil
			})
			httpServer := httptest.NewServer(server.NewStreamableHTTPServer(mcpServer))
			t.Cleanup(httpServer.Close)
			httpTransport, err := transport.NewStreamableHTTP(httpServer.URL)
			require.NoError(t, err)
			mcpClient := client.NewClient(httpTransport, client.WithProtocolVersion(version))
			t.Cleanup(func() { require.NoError(t, mcpClient.Close()) })
			require.NoError(t, mcpClient.Start(t.Context()))
			_, err = mcpClient.Initialize(t.Context(), mcp.InitializeRequest{})
			require.NoError(t, err)
			require.Equal(t, version, mcpClient.ProtocolVersion())
			require.Empty(t, mcpClient.GetServerCapabilities().Extensions)
			for _, value := range []string{"first", "", "second"} {
				request := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "export"}}
				if value != "" {
					request.Params.Meta = &mcp.Meta{AdditionalFields: map[string]any{
						applicationKey:           map[string]any{"session": value, "any nested field": true},
						"com.example/request-id": value,
					}}
				}
				result, err := mcpClient.CallTool(t.Context(), request)
				require.NoError(t, err)
				require.False(t, result.IsError)
				metadata := <-observed
				if value == "" {
					require.NotContains(t, metadata, applicationKey)
					require.NotContains(t, metadata, "com.example/request-id")
				} else {
					require.Equal(t, map[string]any{"session": value, "any nested field": true}, metadata[applicationKey])
					require.Equal(t, value, metadata["com.example/request-id"])
				}
			}
		})
	}
}

func TestValuesOfAbsentMetadata(t *testing.T) {
	require.Nil(t, metadatamcpgo.ValuesOf(mcp.CallToolRequest{}))
}
