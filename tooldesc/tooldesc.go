package tooldesc

import (
	"encoding/json"
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
// then a blank line and details. It panics when [Summary] refuses summary,
// as the tool is built: any test that builds the server fails.
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
	s, err := trimmedSummary(summary)
	if err != nil {
		return "", err
	}
	if details = strings.TrimSpace(details); details == "" {
		return s, nil
	}
	return s + "\n\n" + details, nil
}

// Summary refuses a summary that muster's discovery would not show whole:
// empty, more than one line, or over [SummaryMax] runes.
func Summary(summary string) error {
	_, err := trimmedSummary(summary)
	return err
}

func trimmedSummary(summary string) (string, error) {
	s := strings.TrimSpace(summary)
	n := utf8.RuneCountInString(s)
	switch {
	case n == 0:
		return "", errors.New("tool summary is empty")
	case strings.Contains(s, "\n"):
		return "", fmt.Errorf("tool summary %q spans lines: discovery shows the first line only", s)
	case n > SummaryMax:
		return "", fmt.Errorf("tool summary %q is %d runes, over %d: discovery shows no more", s, n, SummaryMax)
	}
	return s, nil
}

// Budget is what [Check] holds descriptions to, in runes. A zero field is not
// checked.
type Budget struct {
	// Description caps a whole description, summary and details.
	Description int
	// Param caps every description in the input schema: each parameter's,
	// and those of nested properties and array items.
	Param int
	// Prefix is how many leading runes no two tools may share, compared
	// without case and leading space; a description shorter than Prefix is
	// compared whole. It catches a shared boilerplate opening that fills
	// most of the summary, not a shared first word or two.
	Prefix int
}

// DefaultBudget is the budget of giantswarm/giantswarm#38592.
var DefaultBudget = Budget{Description: 600, Param: 150, Prefix: 40}

// Check holds tools to b, and every description's first line to
// [SummaryMax] and non-empty — including a tool that does not use
// [Describe]. It returns every violation, ordered by tool name, or nil.
func Check(tools []mcp.Tool, b Budget) error {
	tools = append([]mcp.Tool(nil), tools...)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	var errs []error
	prefixes := map[string]string{}
	for _, tool := range tools {
		d := tool.Description
		first, _, _ := strings.Cut(d, "\n")
		switch n := utf8.RuneCountInString(strings.TrimSpace(first)); {
		case n == 0:
			errs = append(errs, fmt.Errorf("%s: first line is empty: discovery shows no summary", tool.Name))
		case n > SummaryMax:
			errs = append(errs, fmt.Errorf("%s: first line is %d runes, over %d", tool.Name, n, SummaryMax))
		}
		if b.Description > 0 {
			if n := utf8.RuneCountInString(d); n > b.Description {
				errs = append(errs, fmt.Errorf("%s: description is %d runes, over %d", tool.Name, n, b.Description))
			}
		}
		if p := prefix(d, b.Prefix); p != "" {
			if other, ok := prefixes[p]; ok {
				errs = append(errs, fmt.Errorf("%s: starts like %s: %q", tool.Name, other, p))
			} else {
				prefixes[p] = tool.Name
			}
		}
		if b.Param > 0 {
			errs = append(errs, checkSchema(tool, b.Param)...)
		}
	}
	return errors.Join(errs...)
}

// prefix is the first n runes of d that two tools may not share; "" when n
// is 0 or d is empty.
func prefix(d string, n int) string {
	d = strings.ToLower(strings.TrimSpace(d))
	if n <= 0 || d == "" {
		return ""
	}
	for i := range d {
		if n == 0 {
			return d[:i]
		}
		n--
	}
	return d
}

// checkSchema holds every description in tool's input schema to limit. It
// reads the schema as the tool marshals it, so a structured InputSchema and a
// RawInputSchema (mcp.WithInputSchema, mcp.NewToolWithRawSchema) are read
// alike.
func checkSchema(tool mcp.Tool, limit int) []error {
	b, err := json.Marshal(tool)
	if err != nil {
		return []error{fmt.Errorf("%s: input schema: %w", tool.Name, err)}
	}
	var t struct {
		InputSchema any `json:"inputSchema"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return []error{fmt.Errorf("%s: input schema: %w", tool.Name, err)}
	}
	return walkSchema(tool.Name, "", t.InputSchema, limit)
}

// walkSchema checks the description of node and of everything below it.
// path names a parameter as tool.name, a nested property as tool.name.field
// and an array's items as tool.name[]; other keywords ($defs, anyOf, …)
// appear as themselves.
func walkSchema(tool, path string, node any, limit int) []error {
	var errs []error
	switch v := node.(type) {
	case map[string]any:
		if d, ok := v["description"].(string); ok {
			if n := utf8.RuneCountInString(d); n > limit {
				at := tool
				if path != "" {
					at += "." + path
				}
				errs = append(errs, fmt.Errorf("%s: description is %d runes, over %d", at, n, limit))
			}
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch k {
			case "description":
			case "properties":
				props, _ := v[k].(map[string]any)
				names := make([]string, 0, len(props))
				for name := range props {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					errs = append(errs, walkSchema(tool, join(path, name), props[name], limit)...)
				}
			case "items":
				errs = append(errs, walkSchema(tool, path+"[]", v[k], limit)...)
			default:
				errs = append(errs, walkSchema(tool, join(path, k), v[k], limit)...)
			}
		}
	case []any:
		for i, e := range v {
			errs = append(errs, walkSchema(tool, fmt.Sprintf("%s[%d]", path, i), e, limit)...)
		}
	}
	return errs
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// CheckServer is [Check] over the tools registered on s with AddTool or
// AddTools. Session tools (AddSessionTool) are not among them: pass those to
// [Check].
func CheckServer(s *server.MCPServer, b Budget) error {
	var tools []mcp.Tool
	for _, st := range s.ListTools() {
		tools = append(tools, st.Tool)
	}
	return Check(tools, b)
}
