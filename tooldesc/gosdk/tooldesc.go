// Package gosdk checks and renders official Go SDK tool descriptions.
package gosdk

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/giantswarm/mcp-toolkit/internal/tooldoc"
)

// SummaryMax is the first-line discovery budget in runes.
const SummaryMax = tooldoc.SummaryMax

// Budget limits tool and parameter descriptions.
type Budget = tooldoc.Budget

// DefaultBudget bounds descriptions and distinct summary prefixes.
var DefaultBudget = tooldoc.DefaultBudget

// Describe joins descriptions and panics when the summary violates its budget.
func Describe(summary, details string) string {
	description, err := Description(summary, details)
	if err != nil {
		panic(err)
	}
	return description
}

// Description joins summary and details with a blank line after validation.
func Description(summary, details string) (string, error) {
	return tooldoc.Description(summary, details)
}

// Check reports all description and schema budget violations in name order.
func Check(tools []*mcp.Tool, budget Budget) error {
	return tooldoc.Check(documentedTools(tools), budget)
}

// RenderDoc renders registered descriptions and schemas as a Markdown page.
func RenderDoc(tools []*mcp.Tool, header string) (string, error) {
	return tooldoc.RenderDoc(documentedTools(tools), header)
}

func documentedTools(tools []*mcp.Tool) []tooldoc.Tool {
	documented := make([]tooldoc.Tool, 0, len(tools))
	for _, tool := range tools {
		doc := tooldoc.Tool{Name: tool.Name, Description: tool.Description, SchemaJSON: func() ([]byte, error) {
			encoded, err := json.Marshal(tool.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("%s: input schema: %w", tool.Name, err)
			}
			return encoded, nil
		}}
		if tool.Annotations != nil {
			doc.ReadOnly, doc.Destructive = tool.Annotations.ReadOnlyHint, tool.Annotations.DestructiveHint
		}
		documented = append(documented, doc)
	}
	return documented
}
