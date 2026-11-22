package tooldoc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

type Tool struct {
	Name        string
	Description string
	ReadOnly    bool
	Destructive *bool
	RawSchema   bool
	SchemaJSON  func() ([]byte, error)
}

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
func RenderDoc(tools []Tool, header string) (string, error) {
	tools = sortedByName(tools)
	var b strings.Builder
	b.WriteString(strings.TrimRight(header, "\n"))
	b.WriteString("\n\n| Tool | Kind |\n|---|---|\n")
	for _, tool := range tools {
		fmt.Fprintf(&b, "| `%s` | %s |\n", tool.Name, kind(tool))
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

// sortedByName is a sorted copy of tools: by name, then by description, so
// the order is the same on every run.
func sortedByName(tools []Tool) []Tool {
	tools = slices.Clone(tools)
	slices.SortStableFunc(tools, func(left, right Tool) int {
		if order := strings.Compare(left.Name, right.Name); order != 0 {
			return order
		}
		return strings.Compare(left.Description, right.Description)
	})
	return tools
}

// kind reads a tool's annotations with the MCP defaults for unset hints:
// not read-only, and destructive.
func kind(tool Tool) string {
	if tool.ReadOnly {
		return "read-only"
	}
	if tool.Destructive == nil || *tool.Destructive {
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

// inputSchema is the tool's input schema decoded, numbers as json.Number.
func inputSchema(tool Tool) (any, error) {
	b, err := tool.SchemaJSON()
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
// structured one is decoded and encoded again with sorted keys and no HTML escapes.
func indentedSchema(tool Tool) (string, error) {
	var buf bytes.Buffer
	if tool.RawSchema {
		raw, err := tool.SchemaJSON()
		if err != nil {
			return "", err
		}
		if err := json.Indent(&buf, raw, "", "  "); err != nil {
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
