package tooldesc

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-toolkit/internal/tooldoc"
)

// RenderDoc renders registered descriptions and schemas as a Markdown page.
func RenderDoc(tools []mcp.Tool, header string) (string, error) {
	return tooldoc.RenderDoc(documentedTools(tools), header)
}

// RenderServerDoc renders the tools registered with AddTool or AddTools.
func RenderServerDoc(target *server.MCPServer, header string) (string, error) {
	return RenderDoc(serverTools(target), header)
}

func serverTools(target *server.MCPServer) []mcp.Tool {
	tools := make([]mcp.Tool, 0, len(target.ListTools()))
	for _, registered := range target.ListTools() {
		tools = append(tools, registered.Tool)
	}
	return tools
}

func documentedTools(tools []mcp.Tool) []tooldoc.Tool {
	documented := make([]tooldoc.Tool, 0, len(tools))
	for _, tool := range tools {
		documented = append(documented, tooldoc.Tool{
			Name: tool.Name, Description: tool.Description,
			ReadOnly:    tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint,
			Destructive: tool.Annotations.DestructiveHint,
			RawSchema:   tool.RawInputSchema != nil,
			SchemaJSON: func() ([]byte, error) {
				if tool.RawInputSchema != nil {
					return tool.RawInputSchema, nil
				}
				encoded, err := json.Marshal(tool.InputSchema)
				if err != nil {
					return nil, fmt.Errorf("%s: input schema: %w", tool.Name, err)
				}
				return encoded, nil
			},
		})
	}
	return documented
}
