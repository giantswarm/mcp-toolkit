package tooldesctest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/tooldesc"
	"github.com/giantswarm/mcp-toolkit/tooldesc/tooldesctest"
)

const header = "# The tools"

// recorder is a testing.TB that records failures instead of failing.
type recorder struct {
	testing.TB
	errors, fatals []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}

func newServer() *server.MCPServer {
	s := server.NewMCPServer("test", "0")
	s.AddTool(mcp.NewTool("list_pods", tooldesc.Describe("List pods.", "")), nil)
	return s
}

func TestAssertServerDoc_WritesThenPasses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.md")
	t.Setenv(tooldesctest.UpdateEnv, "1")
	tooldesctest.AssertServerDoc(t, newServer(), header, path)
	want, err := tooldesc.RenderServerDoc(newServer(), header)
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	require.Equal(t, want, string(got))

	t.Setenv(tooldesctest.UpdateEnv, "")
	tooldesctest.AssertServerDoc(t, newServer(), header, path)
}

func TestAssertDoc_StalePageNamesTheLineAndTheSwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.md")
	require.NoError(t, os.WriteFile(path, []byte("# The tools\n\nold\n"), 0o600))
	r := &recorder{TB: t}
	tooldesctest.AssertDoc(r, []mcp.Tool{mcp.NewTool("list_pods", tooldesc.Describe("List pods.", ""))}, header, path)
	require.Empty(t, r.fatals)
	require.Equal(t, []string{path + " is out of date at line 3: run the test with TOOLS_DOC_UPDATE=1 to rewrite it"}, r.errors)
}

func TestAssertDoc_MissingPageNamesTheSwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.md")
	r := &recorder{TB: t}
	tooldesctest.AssertDoc(r, nil, header, path)
	require.Equal(t, []string{path + " does not exist: run the test with TOOLS_DOC_UPDATE=1 to write it"}, r.fatals)
}
