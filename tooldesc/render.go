package tooldesc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RenderDoc renders tools as the markdown page a server keeps as
// docs/tools.md, so reviewers read the exact text a model gets: header as
// given, a table of the tools with their kind from the annotations
// (read-only, write, or write, destructive), then per tool its description
// and its input schema as indented JSON. Tools are ordered by name.
//
// The page renders on GitHub as the model reads it: a < outside a code span
// is escaped, so <namespace> is not taken for an HTML tag, and the schemas
// are written without JSON's HTML escapes (< for <).
func RenderDoc(tools []mcp.Tool, header string) (string, error) {
	tools = append([]mcp.Tool(nil), tools...)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	var b strings.Builder
	b.WriteString(strings.TrimRight(header, "\n"))
	b.WriteString("\n\n| Tool | Kind |\n|---|---|\n")
	for _, tool := range tools {
		fmt.Fprintf(&b, "| `%s` | %s |\n", tool.Name, kind(tool.Annotations))
	}
	for _, tool := range tools {
		schema, err := indentedSchema(tool)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\n## `%s`\n\n%s\n\n```json\n%s```\n", tool.Name, escapeMarkdown(tool.Description), schema)
	}
	return b.String(), nil
}

// RenderServerDoc is [RenderDoc] over the tools registered on s with AddTool
// or AddTools.
func RenderServerDoc(s *server.MCPServer, header string) (string, error) {
	var tools []mcp.Tool
	for _, st := range s.ListTools() {
		tools = append(tools, st.Tool)
	}
	return RenderDoc(tools, header)
}

// kind reads a tool's annotations with the MCP defaults for unset hints:
// not read-only, and destructive.
func kind(a mcp.ToolAnnotation) string {
	if a.ReadOnlyHint != nil && *a.ReadOnlyHint {
		return "read-only"
	}
	if a.DestructiveHint == nil || *a.DestructiveHint {
		return "write, destructive"
	}
	return "write"
}

// indentedSchema is the tool's input schema as indented JSON with a trailing
// newline, without HTML escapes.
func indentedSchema(tool mcp.Tool) (string, error) {
	schema, err := inputSchema(tool)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		return "", fmt.Errorf("%s: input schema: %w", tool.Name, err)
	}
	return buf.String(), nil
}

// inputSchema is the tool's input schema as the tool marshals it, so a
// structured InputSchema and a RawInputSchema (mcp.WithInputSchema,
// mcp.NewToolWithRawSchema) read alike. mcp-go's own marshalling escapes <, >
// and &; decoded, they are themselves again.
func inputSchema(tool mcp.Tool) (any, error) {
	b, err := json.Marshal(tool)
	if err != nil {
		return nil, fmt.Errorf("%s: input schema: %w", tool.Name, err)
	}
	var t struct {
		InputSchema any `json:"inputSchema"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("%s: input schema: %w", tool.Name, err)
	}
	return t.InputSchema, nil
}

// escapeMarkdown escapes every < outside a code span. Inside one a backslash
// shows literally, and a code span already keeps < as text.
func escapeMarkdown(s string) string {
	var b strings.Builder
	open := 0 // length of the backtick run that opened the current code span
	for i := 0; i < len(s); {
		if s[i] == '`' {
			n := 0
			for i+n < len(s) && s[i+n] == '`' {
				n++
			}
			switch {
			case open == 0 && hasRun(s[i+n:], n):
				open = n
			case open == n:
				open = 0
			}
			b.WriteString(s[i : i+n])
			i += n
			continue
		}
		if s[i] == '<' && open == 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// hasRun says whether s holds a run of exactly n backticks, the run that
// closes a code span opened by n.
func hasRun(s string, n int) bool {
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		m := 0
		for i+m < len(s) && s[i+m] == '`' {
			m++
		}
		if m == n {
			return true
		}
		i += m
	}
	return false
}
