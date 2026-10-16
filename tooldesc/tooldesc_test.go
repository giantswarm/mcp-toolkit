package tooldesc_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/tooldesc"
)

func TestDescription_JoinsWithABlankLine(t *testing.T) {
	d, err := tooldesc.Description(" List pods. ", " Details. ")
	require.NoError(t, err)
	require.Equal(t, "List pods.\n\nDetails.", d)
}

func TestDescription_EmptyDetailsLeaveTheSummary(t *testing.T) {
	d, err := tooldesc.Description("List pods.", "  ")
	require.NoError(t, err)
	require.Equal(t, "List pods.", d)
}

func TestSummary_FitsExactlySummaryMax(t *testing.T) {
	require.NoError(t, tooldesc.Summary(strings.Repeat("é", tooldesc.SummaryMax)))
}

func TestSummary_Refusals(t *testing.T) {
	for name, summary := range map[string]string{
		"empty":     " ",
		"two lines": "List pods.\nAnd more.",
		"too long":  strings.Repeat("x", tooldesc.SummaryMax+1),
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, tooldesc.Summary(summary))
		})
	}
}

func TestDescribe_PanicsOnARefusedSummary(t *testing.T) {
	require.Panics(t, func() { tooldesc.Describe(strings.Repeat("x", tooldesc.SummaryMax+1), "") })
}

func TestDescribe_SetsTheDescription(t *testing.T) {
	tool := mcp.NewTool("list_pods", tooldesc.Describe("List pods.", "Details."))
	require.Equal(t, "List pods.\n\nDetails.", tool.Description)
}

func TestCheck_PassesWithinBudget(t *testing.T) {
	tools := []mcp.Tool{
		mcp.NewTool("list_pods", tooldesc.Describe("List pods in a namespace.", "Details."),
			mcp.WithString("namespace", mcp.Description("The namespace."))),
		mcp.NewTool("get_pod", tooldesc.Describe("Get one pod.", "")),
	}
	require.NoError(t, tooldesc.Check(tools, tooldesc.DefaultBudget))
}

func TestCheck_ReportsEveryViolation(t *testing.T) {
	boilerplate := "WRITES (as you, with your own token through the App). "
	tools := []mcp.Tool{
		mcp.NewTool("create", mcp.WithDescription(boilerplate+"Create a thing.")),
		mcp.NewTool("delete", mcp.WithDescription(boilerplate+"Delete a thing."),
			mcp.WithString("confirm", mcp.Description(strings.Repeat("y", 151)))),
		mcp.NewTool("long", mcp.WithDescription(strings.Repeat("z", 601))),
	}
	err := tooldesc.Check(tools, tooldesc.DefaultBudget)
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "delete: starts like create")
	require.Contains(t, msg, "delete.confirm: description is 151 runes, over 150")
	require.Contains(t, msg, "long: first line is 601 runes, over 120")
	require.Contains(t, msg, "long: description is 601 runes, over 600")
}

func TestCheck_ZeroBudgetChecksTheSummaryLineOnly(t *testing.T) {
	tools := []mcp.Tool{
		mcp.NewTool("a", mcp.WithDescription("Same opening for both tools, then more text.\n\n"+strings.Repeat("z", 700))),
		mcp.NewTool("b", mcp.WithDescription("Same opening for both tools, then more text.")),
	}
	require.NoError(t, tooldesc.Check(tools, tooldesc.Budget{}))
}

func TestCheckServer_ReadsTheRegisteredTools(t *testing.T) {
	s := server.NewMCPServer("test", "0")
	s.AddTool(mcp.NewTool("long", mcp.WithDescription(strings.Repeat("z", 121))), nil)
	require.ErrorContains(t, tooldesc.CheckServer(s, tooldesc.Budget{}), "long: first line is 121 runes")
}

func TestCheck_OrdersViolationsByToolName(t *testing.T) {
	long := strings.Repeat("z", 121)
	tools := []mcp.Tool{
		mcp.NewTool("zeta", mcp.WithDescription(long+"a")),
		mcp.NewTool("alpha", mcp.WithDescription(long+"b")),
		mcp.NewTool("mid", mcp.WithDescription(long+"c")),
	}
	err := tooldesc.Check(tools, tooldesc.Budget{})
	require.EqualError(t, err, strings.Join([]string{
		"alpha: first line is 122 runes, over 120",
		"mid: first line is 122 runes, over 120",
		"zeta: first line is 122 runes, over 120",
	}, "\n"))
}

func TestCheck_EmptyFirstLine(t *testing.T) {
	for name, d := range map[string]string{"no description": "", "leading newline": "\nDelete a repository."} {
		t.Run(name, func(t *testing.T) {
			err := tooldesc.Check([]mcp.Tool{mcp.NewTool("delete_repo", mcp.WithDescription(d))}, tooldesc.Budget{})
			require.ErrorContains(t, err, "delete_repo: first line is empty")
		})
	}
}

func TestCheck_ShortIdenticalDescriptionsShareThePrefix(t *testing.T) {
	tools := []mcp.Tool{
		mcp.NewTool("a", mcp.WithDescription("Read-only. List the things.")),
		mcp.NewTool("b", mcp.WithDescription("  read-only. list the things.")),
	}
	require.ErrorContains(t, tooldesc.Check(tools, tooldesc.DefaultBudget), `b: starts like a: "read-only. list the things."`)
}

func TestCheck_ShortSharedOpeningIsAllowed(t *testing.T) {
	tools := []mcp.Tool{
		mcp.NewTool("list_pods", mcp.WithDescription("List pods in a namespace, filtered by label and phase.")),
		mcp.NewTool("list_nodes", mcp.WithDescription("List nodes of the cluster, with their capacity and taints.")),
	}
	require.NoError(t, tooldesc.Check(tools, tooldesc.DefaultBudget))
}

func TestCheck_NestedSchemaDescriptions(t *testing.T) {
	long := strings.Repeat("y", 151)
	tool := mcp.NewTool("update", tooldesc.Describe("Update an entry.", ""),
		mcp.WithObject("entry", mcp.Description("The entry."), mcp.Properties(map[string]any{
			"lifecycle":   map[string]any{"type": "string", "description": long},
			"description": map[string]any{"type": "string", "description": long},
		})),
		mcp.WithArray("tags", mcp.Items(map[string]any{"type": "string", "description": long})),
	)
	err := tooldesc.Check([]mcp.Tool{tool}, tooldesc.DefaultBudget)
	require.EqualError(t, err, strings.Join([]string{
		"update.entry.description: description is 151 runes, over 150",
		"update.entry.lifecycle: description is 151 runes, over 150",
		"update.tags[]: description is 151 runes, over 150",
	}, "\n"))
}

func TestCheck_RawInputSchema(t *testing.T) {
	raw := []byte(`{"type":"object","properties":{"confirm":{"type":"string","description":"` + strings.Repeat("y", 151) + `"}}}`)
	tool := mcp.NewToolWithRawSchema("delete", "Delete a thing.", raw)
	require.ErrorContains(t, tooldesc.Check([]mcp.Tool{tool}, tooldesc.DefaultBudget), "delete.confirm: description is 151 runes, over 150")
}

type deleteArgs struct {
	Confirm string `json:"confirm" jsonschema:"the repository name exactly as the person typed it; ask them and never fill it in yourself, because a wrong name deletes another repository, which an org owner must restore"`
}

func TestCheck_TypedInputSchema(t *testing.T) {
	tool := mcp.NewTool("delete", tooldesc.Describe("Delete a thing.", ""), mcp.WithInputSchema[deleteArgs]())
	require.ErrorContains(t, tooldesc.Check([]mcp.Tool{tool}, tooldesc.DefaultBudget), "delete.confirm: description is")
}
