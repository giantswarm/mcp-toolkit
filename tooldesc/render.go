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
// Each description is a fenced text block, so no markdown in it is
// interpreted: GitHub shows the characters the model reads. Schemas keep
// their numbers as written and are free of JSON's HTML escapes (< for
// <); a raw schema keeps its key order.
func RenderDoc(tools []mcp.Tool, header string) (string, error) {
	tools = sortedByName(tools)
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
		fmt.Fprintf(&b, "\n## `%s`\n\n%s\n\n```json\n%s```\n", tool.Name, fenced(tool.Description), schema)
	}
	return b.String(), nil
}

// RenderServerDoc is [RenderDoc] over the tools registered on s with AddTool
// or AddTools.
func RenderServerDoc(s *server.MCPServer, header string) (string, error) {
	return RenderDoc(serverTools(s), header)
}

// serverTools are the tools registered on s with AddTool or AddTools.
func serverTools(s *server.MCPServer) []mcp.Tool {
	var tools []mcp.Tool
	for _, st := range s.ListTools() {
		tools = append(tools, st.Tool)
	}
	return tools
}

// sortedByName is a sorted copy of tools: by name, then by description, so
// the order is the same on every run.
func sortedByName(tools []mcp.Tool) []mcp.Tool {
	tools = append([]mcp.Tool(nil), tools...)
	sort.SliceStable(tools, func(i, j int) bool {
		if tools[i].Name != tools[j].Name {
			return tools[i].Name < tools[j].Name
		}
		return tools[i].Description < tools[j].Description
	})
	return tools
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

// fenced puts s in a fenced text block whose fence is longer than any run of
// backticks in s, so nothing in s can close it.
func fenced(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + "text\n" + s + "\n" + fence
}

// schemaJSON is the tool's input schema as the tool sends it: a
// RawInputSchema (mcp.WithInputSchema, mcp.NewToolWithRawSchema) as given,
// else the structured InputSchema marshalled.
func schemaJSON(tool mcp.Tool) ([]byte, error) {
	if tool.RawInputSchema != nil {
		return tool.RawInputSchema, nil
	}
	b, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("%s: input schema: %w", tool.Name, err)
	}
	return b, nil
}

// inputSchema is the tool's input schema decoded, numbers as json.Number.
func inputSchema(tool mcp.Tool) (any, error) {
	b, err := schemaJSON(tool)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s: input schema: %w", tool.Name, err)
	}
	return v, nil
}

// indentedSchema is the tool's input schema as indented JSON with a trailing
// newline. A raw schema is indented as written, its key order kept; a
// structured one is decoded and encoded again, which undoes mcp-go's HTML
// escapes and writes its keys sorted, as mcp-go does.
func indentedSchema(tool mcp.Tool) (string, error) {
	var buf bytes.Buffer
	if tool.RawInputSchema != nil {
		if err := json.Indent(&buf, tool.RawInputSchema, "", "  "); err != nil {
			return "", fmt.Errorf("%s: input schema: %w", tool.Name, err)
		}
		buf.WriteByte('\n')
		return buf.String(), nil
	}
	schema, err := inputSchema(tool)
	if err != nil {
		return "", err
	}
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		return "", fmt.Errorf("%s: input schema: %w", tool.Name, err)
	}
	return buf.String(), nil
}
