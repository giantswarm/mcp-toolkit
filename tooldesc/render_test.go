package tooldesc_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/tooldesc"
)

const header = "# The tools\n\nGenerated from the registered tools."

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
	require.Equal(t, header+`

| Tool | Kind |
|---|---|
| `+"`get_pod`"+` | read-only |
| `+"`list_pods`"+` | read-only |

## `+"`get_pod`"+`

Get one pod.

`+"```json"+`
{
  "properties": {},
  "required": [],
  "type": "object"
}
`+"```"+`

## `+"`list_pods`"+`

List pods.

Details.

`+"```json"+`
{
  "properties": {
    "namespace": {
      "description": "The namespace.",
      "type": "string"
    }
  },
  "required": [],
  "type": "object"
}
`+"```"+`
`, doc)
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

func TestRenderDoc_EscapesAngleBracketsOutsideCode(t *testing.T) {
	doc := render(t, mcp.NewTool("start",
		mcp.WithDescription("Send it to tenant <namespace>/<name>; the file is `repositories/<team>.yaml`, or ``a ` <b>``, not `<open."),
		mcp.WithString("origin", mcp.Description("<scheme>://<host> & more"))))
	require.Contains(t, doc, "Send it to tenant \\<namespace>/\\<name>; the file is `repositories/<team>.yaml`, or ``a ` <b>``, not `\\<open.")
	require.Contains(t, doc, `"description": "<scheme>://<host> & more"`)
	require.NotContains(t, doc, `\u003c`)
	require.NotContains(t, doc, `\u0026`)
}

func TestRenderDoc_RawInputSchema(t *testing.T) {
	tool := mcp.NewToolWithRawSchema("raw", "A raw schema.", []byte(`{"type":"object","properties":{"q":{"type":"string","description":"<query>"}}}`))
	require.Contains(t, render(t, tool), `"description": "<query>"`)
}

func TestRenderServerDoc_ReadsTheRegisteredTools(t *testing.T) {
	s := server.NewMCPServer("test", "0")
	s.AddTool(mcp.NewTool("b_tool", tooldesc.Describe("B.", "")), nil)
	s.AddTool(mcp.NewTool("a_tool", tooldesc.Describe("A.", "")), nil)
	doc, err := tooldesc.RenderServerDoc(s, header)
	require.NoError(t, err)
	require.Less(t, strings.Index(doc, "## `a_tool`"), strings.Index(doc, "## `b_tool`"))
}
