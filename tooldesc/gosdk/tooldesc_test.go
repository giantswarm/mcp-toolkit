package gosdk_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/tooldesc/gosdk"
)

func TestNativeDescriptionAndSchemaDocumentation(t *testing.T) {
	tool := &mcp.Tool{Name: "read", Description: gosdk.Describe("Read a value.", "Return its exact value."), Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}, InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","maximum":9007199254740993,"description":"Maximum value."}}}`)}
	require.NoError(t, gosdk.Check([]*mcp.Tool{tool}, gosdk.DefaultBudget))
	document, err := gosdk.RenderDoc([]*mcp.Tool{tool}, "# Tools")
	require.NoError(t, err)
	require.Contains(t, document, "| `read` | read-only |")
	require.Contains(t, document, "9007199254740993")
	require.Contains(t, document, tool.Description)
	tool.Description = ""
	require.ErrorContains(t, gosdk.Check([]*mcp.Tool{tool}, gosdk.DefaultBudget), "first line is empty")
	require.Panics(t, func() { gosdk.Describe(strings.Repeat("x", gosdk.SummaryMax+1), "") })
}
