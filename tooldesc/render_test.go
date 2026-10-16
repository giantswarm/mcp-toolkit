package tooldesc_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/tooldesc"
)

const (
	header = "# The tools\n\nGenerated from the registered tools."
	fence3 = "```"
)

// jsonEscape is a JSON HTML escape such as \u003c, built so no tool decodes it.
func jsonEscape(hex string) string { return string(rune(92)) + "u" + hex }

func render(t *testing.T, tools ...mcp.Tool) string {
	t.Helper()
	doc, err := tooldesc.RenderDoc(tools, header)
	require.NoError(t, err)
	return doc
}

func TestRenderDoc_Page(t *testing.T) {
	doc := render(t,
		mcp.NewTool("list_pods", tooldesc.Describe("List pods.", "Details."), mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("namespace", mcp.Description("The namespace."))),
		mcp.NewTool("get_pod", tooldesc.Describe("Get one pod.", ""), mcp.WithReadOnlyHintAnnotation(true)),
	)
	want := header + "\n\n" +
		"| Tool | Kind |\n|---|---|\n| `get_pod` | read-only |\n| `list_pods` | read-only |\n" +
		"\n## `get_pod`\n\n" + fence3 + "text\nGet one pod.\n" + fence3 + "\n\n" +
		fence3 + "json\n{\n  \"properties\": {},\n  \"required\": [],\n  \"type\": \"object\"\n}\n" + fence3 + "\n" +
		"\n## `list_pods`\n\n" + fence3 + "text\nList pods.\n\nDetails.\n" + fence3 + "\n\n" +
		fence3 + "json\n{\n  \"properties\": {\n    \"namespace\": {\n      \"description\": \"The namespace.\",\n      \"type\": \"string\"\n    }\n  },\n  \"required\": [],\n  \"type\": \"object\"\n}\n" + fence3 + "\n"
	require.Equal(t, want, doc)
}

func TestRenderDoc_KindFromAnnotations(t *testing.T) {
	unset := mcp.Tool{Name: "unset", Description: "No hints at all."}
	doc := render(t,
		mcp.NewTool("read", mcp.WithDescription("Reads."), mcp.WithReadOnlyHintAnnotation(true)),
		mcp.NewTool("add", mcp.WithDescription("Adds."), mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false)),
		mcp.NewTool("remove", mcp.WithDescription("Removes."), mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(true)),
		unset,
	)
	for _, row := range []string{"| `add` | write |", "| `read` | read-only |", "| `remove` | write, destructive |", "| `unset` | write, destructive |"} {
		require.Contains(t, doc, row+"\n")
	}
}

func TestRenderDoc_DescriptionsAreVerbatim(t *testing.T) {
	desc := "Tenant <namespace>/<name>, *all*, &lt; and \\<x>.\n\n    kubectl get <pod>\n\n" +
		"An unclosed " + fence3 + "yaml\nkey: <v>"
	doc := render(t, mcp.NewTool("start", mcp.WithDescription(desc)))
	require.Contains(t, doc, "````text\n"+desc+"\n````\n", "a fence longer than the description's backtick runs")
	require.Contains(t, doc, "\n\n"+fence3+"json\n", "the schema's fence is not swallowed")
}

func TestRenderDoc_SchemasAsSent(t *testing.T) {
	structured := mcp.NewTool("origin", mcp.WithDescription("Origins."),
		mcp.WithString("origin", mcp.Description("<scheme>://<host> & more")))
	raw := mcp.NewToolWithRawSchema("raw", "A raw schema.",
		[]byte(`{"type":"object","properties":{"z":{"type":"integer","maximum":9007199254740993},"a":{"type":"number","minimum":1e21,"description":"<query>"}}}`))
	doc := render(t, structured, raw)
	require.Contains(t, doc, `"description": "<scheme>://<host> & more"`)
	require.NotContains(t, doc, jsonEscape("003c"))
	require.NotContains(t, doc, jsonEscape("0026"))
	require.Contains(t, doc, `"maximum": 9007199254740993`)
	require.Contains(t, doc, `"minimum": 1e21`)
	require.Less(t, strings.Index(doc, `"z": {`), strings.Index(doc, `"a": {`), "a raw schema keeps its key order")
}

func TestRenderDoc_StructuredSchemaKeepsExactNumbers(t *testing.T) {
	tool := mcp.NewTool("limits", mcp.WithDescription("Limits."),
		mcp.WithObject("page", mcp.Properties(map[string]any{"size": map[string]any{"type": "integer", "maximum": int64(9007199254740993)}})))
	require.Contains(t, render(t, tool), `"maximum": 9007199254740993`)
}

func TestRenderDoc_SameNameOrderedByDescription(t *testing.T) {
	a := mcp.Tool{Name: "x", Description: "a"}
	b := mcp.Tool{Name: "x", Description: "b"}
	require.Equal(t, render(t, a, b), render(t, b, a))
}

func TestRenderServerDoc_ReadsTheRegisteredTools(t *testing.T) {
	s := server.NewMCPServer("test", "0")
	s.AddTool(mcp.NewTool("b_tool", tooldesc.Describe("B.", "")), nil)
	s.AddTool(mcp.NewTool("a_tool", tooldesc.Describe("A.", "")), nil)
	doc, err := tooldesc.RenderServerDoc(s, header)
	require.NoError(t, err)
	require.Less(t, strings.Index(doc, "## `a_tool`"), strings.Index(doc, "## `b_tool`"))
}
