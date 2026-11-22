package gosdk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func limitResults(maxBytes int) func(mcp.ToolHandler) mcp.ToolHandler {
	return func(next mcp.ToolHandler) mcp.ToolHandler {
		return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := next(ctx, request)
			if err != nil || result == nil || maxBytes <= 0 {
				return result, err
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("tool result cannot be encoded")
			}
			if len(encoded) > maxBytes {
				return toolFailure(fmt.Sprintf("tool result exceeds %d bytes", maxBytes)), nil
			}
			return result, nil
		}
	}
}
