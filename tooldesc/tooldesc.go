package tooldesc

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-toolkit/internal/tooldoc"
)

// SummaryMax is the first-line discovery budget in runes.
const SummaryMax = tooldoc.SummaryMax

// Budget limits tool and parameter descriptions.
type Budget = tooldoc.Budget

// DefaultBudget bounds descriptions and distinct summary prefixes.
var DefaultBudget = tooldoc.DefaultBudget

// Describe sets the description and panics when Summary refuses the summary.
func Describe(summary, details string) mcp.ToolOption {
	description, err := Description(summary, details)
	if err != nil {
		panic(err)
	}
	return mcp.WithDescription(description)
}

// Description joins summary and details with a blank line after validation.
func Description(summary, details string) (string, error) {
	return tooldoc.Description(summary, details)
}

// Summary validates the first-line discovery budget.
func Summary(summary string) error { return tooldoc.Summary(summary) }

// Check reports all description and schema budget violations in name order.
func Check(tools []mcp.Tool, budget Budget) error {
	return tooldoc.Check(documentedTools(tools), budget)
}

// CheckServer checks tools registered with AddTool or AddTools.
func CheckServer(target *server.MCPServer, budget Budget) error {
	return Check(serverTools(target), budget)
}
