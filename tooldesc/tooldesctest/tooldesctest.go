// Package tooldesctest keeps a server's docs/tools.md current in its tests:
// the page [tooldesc.RenderDoc] renders, compared with the file, and written
// to it when the update switch is set.
//
//	func TestToolsDocIsCurrent(t *testing.T) {
//		tooldesctest.AssertServerDoc(t, newServer(), "# The tools", "../../docs/tools.md")
//	}
//
// Run the test with TOOLS_DOC_UPDATE=1 to write the page.
package tooldesctest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/giantswarm/mcp-toolkit/tooldesc"
)

// UpdateEnv is the environment variable that makes the assertions write the
// page instead of only comparing it.
const UpdateEnv = "TOOLS_DOC_UPDATE"

// AssertServerDoc is [AssertDoc] over the tools registered on s with AddTool
// or AddTools.
func AssertServerDoc(t testing.TB, s *server.MCPServer, header, path string) {
	t.Helper()
	got, err := tooldesc.RenderServerDoc(s, header)
	assertDoc(t, got, err, path)
}

// AssertDoc fails t unless the file at path holds the page [tooldesc.RenderDoc]
// renders for tools and header. With [UpdateEnv] set it writes the page
// first.
func AssertDoc(t testing.TB, tools []mcp.Tool, header, path string) {
	t.Helper()
	got, err := tooldesc.RenderDoc(tools, header)
	assertDoc(t, got, err, path)
}

func assertDoc(t testing.TB, want string, err error, path string) {
	t.Helper()
	if err != nil {
		t.Fatalf("render %s: %v", path, err)
		return
	}
	if os.Getenv(UpdateEnv) != "" {
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
			return
		}
	}
	got, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s does not exist: run the test with %s=1 to write it", path, UpdateEnv)
		return
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
		return
	}
	if string(got) != want {
		t.Errorf("%s is out of date at line %d: run the test with %s=1 to rewrite it", path, firstDiff(string(got), want), UpdateEnv)
	}
}

// firstDiff is the 1-based number of the first line where a and b differ.
func firstDiff(a, b string) int {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range min(len(al), len(bl)) {
		if al[i] != bl[i] {
			return i + 1
		}
	}
	return min(len(al), len(bl)) + 1
}
