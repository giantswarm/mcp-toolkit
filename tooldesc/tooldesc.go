package tooldesc

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SummaryMax is how much of a description muster's discovery shows: the
// first line, cut to 120 runes (summaryMaxLen in muster's
// internal/metatools/handlers_tools.go).
const SummaryMax = 120

// Describe is the description option of a tool: summary on the first line,
// then a blank line and details. It panics when [Summary] refuses summary.
func Describe(summary, details string) mcp.ToolOption {
	d, err := Description(summary, details)
	if err != nil {
		panic(err)
	}
	return mcp.WithDescription(d)
}

// Description joins summary and details as [Describe] does, or returns why
// [Summary] refuses summary. Empty details leave the summary alone.
func Description(summary, details string) (string, error) {
	if err := Summary(summary); err != nil {
		return "", err
	}
	summary = strings.TrimSpace(summary)
	if details = strings.TrimSpace(details); details == "" {
		return summary, nil
	}
	return summary + "\n\n" + details, nil
}

// Summary refuses a summary that muster's discovery would not show whole:
// empty, more than one line, or over [SummaryMax] runes.
func Summary(summary string) error {
	s := strings.TrimSpace(summary)
	n := utf8.RuneCountInString(s)
	switch {
	case n == 0:
		return errors.New("tool summary is empty")
	case strings.Contains(s, "\n"):
		return fmt.Errorf("tool summary %q spans lines: discovery shows the first line only", s)
	case n > SummaryMax:
		return fmt.Errorf("tool summary %q is %d runes, over %d: discovery shows no more", s, n, SummaryMax)
	}
	return nil
}

// Budget is what [Check] holds descriptions to, in runes. A zero field is not
// checked.
type Budget struct {
	// Description caps a whole description, summary and details.
	Description int
	// Param caps each input parameter's description.
	Param int
	// Prefix is how many leading runes no two tools may share.
	Prefix int
}

// DefaultBudget is the budget of giantswarm/giantswarm#38592.
var DefaultBudget = Budget{Description: 600, Param: 150, Prefix: 40}

// Check holds tools to b, and every description's first line to
// [SummaryMax] — including a tool that does not use [Describe]. It returns
// every violation, ordered by tool name, or nil.
func Check(tools []mcp.Tool, b Budget) error {
	tools = append([]mcp.Tool(nil), tools...)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	var errs []error
	prefixes := map[string]string{}
	for _, tool := range tools {
		d := tool.Description
		first, _, _ := strings.Cut(d, "\n")
		if n := utf8.RuneCountInString(strings.TrimSpace(first)); n > SummaryMax {
			errs = append(errs, fmt.Errorf("%s: first line is %d runes, over %d", tool.Name, n, SummaryMax))
		}
		if n := utf8.RuneCountInString(d); b.Description > 0 && n > b.Description {
			errs = append(errs, fmt.Errorf("%s: description is %d runes, over %d", tool.Name, n, b.Description))
		}
		if r := []rune(d); b.Prefix > 0 && len(r) >= b.Prefix {
			p := string(r[:b.Prefix])
			if other, ok := prefixes[p]; ok {
				errs = append(errs, fmt.Errorf("%s: starts like %s: %q", tool.Name, other, p))
			} else {
				prefixes[p] = tool.Name
			}
		}
		if b.Param > 0 {
			errs = append(errs, checkParams(tool, b.Param)...)
		}
	}
	return errors.Join(errs...)
}

func checkParams(tool mcp.Tool, max int) []error {
	names := make([]string, 0, len(tool.InputSchema.Properties))
	for name := range tool.InputSchema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	var errs []error
	for _, name := range names {
		prop, _ := tool.InputSchema.Properties[name].(map[string]any)
		d, _ := prop["description"].(string)
		if n := utf8.RuneCountInString(d); n > max {
			errs = append(errs, fmt.Errorf("%s.%s: description is %d runes, over %d", tool.Name, name, n, max))
		}
	}
	return errs
}

// CheckServer is [Check] over the tools registered on s.
func CheckServer(s *server.MCPServer, b Budget) error {
	var tools []mcp.Tool
	for _, st := range s.ListTools() {
		tools = append(tools, st.Tool)
	}
	return Check(tools, b)
}
