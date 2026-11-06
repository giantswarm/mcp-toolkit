package brand_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/brand"
)

const dataURIPrefix = "data:image/svg+xml;base64,"

// musterIconLimit is the largest icon src muster accepts.
const musterIconLimit = 64 << 10

func TestGiantSwarmIcon_DataURIDecodesToTheLogo(t *testing.T) {
	want, err := os.ReadFile("giantswarm.svg")
	require.NoError(t, err)

	icon := brand.GiantSwarmIcon()
	require.True(t, strings.HasPrefix(icon.Src, dataURIPrefix))
	got, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(icon.Src, dataURIPrefix))
	require.NoError(t, err)
	require.Equal(t, want, got)

	require.Equal(t, "image/svg+xml", icon.MIMEType)
	require.Equal(t, []string{"any"}, icon.Sizes)
	require.Empty(t, icon.Theme)
}

func TestGiantSwarmIcon_IsWellFormedSVG(t *testing.T) {
	got, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(brand.GiantSwarmIcon().Src, dataURIPrefix))
	require.NoError(t, err)

	var root struct {
		XMLName xml.Name
	}
	require.NoError(t, xml.Unmarshal(got, &root))
	require.Equal(t, "svg", root.XMLName.Local)
	require.Equal(t, "http://www.w3.org/2000/svg", root.XMLName.Space)

	// Unmarshal stops after the root; walk every token to reject trailing
	// malformed markup too.
	dec := xml.NewDecoder(bytes.NewReader(got))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
	}
}

func TestGiantSwarmIcon_FitsMustersLimit(t *testing.T) {
	require.Less(t, len(brand.GiantSwarmIcon().Src), musterIconLimit)
}

func TestIcon_DataURIDecodesToTheSVG(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`)

	icon := brand.Icon(svg)
	require.True(t, strings.HasPrefix(icon.Src, dataURIPrefix))
	got, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(icon.Src, dataURIPrefix))
	require.NoError(t, err)
	require.Equal(t, svg, got)

	require.Equal(t, "image/svg+xml", icon.MIMEType)
	require.Equal(t, []string{"any"}, icon.Sizes)
	require.Empty(t, icon.Theme)
}

func TestWithIcon_InitializeResultCarriesTheIcon(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`)
	require.Equal(t, []mcp.Icon{brand.Icon(svg)}, initializeIcons(t, brand.WithIcon(svg)))
}

func TestWithGiantSwarmIcon_InitializeResultCarriesTheIcon(t *testing.T) {
	require.Equal(t, []mcp.Icon{brand.GiantSwarmIcon()}, initializeIcons(t, brand.WithGiantSwarmIcon()))
}

// initializeIcons returns serverInfo.icons of the initialize result of a
// server built with opt.
func initializeIcons(t *testing.T, opt server.ServerOption) []mcp.Icon {
	t.Helper()
	s := server.NewMCPServer("test", "0", opt)

	resp := s.HandleMessage(context.Background(), json.RawMessage(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "`+mcp.LATEST_PROTOCOL_VERSION+`",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "0"}
		}
	}`))

	rpc, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok, "response: %#v", resp)
	result, ok := rpc.Result.(mcp.InitializeResult)
	require.True(t, ok, "result: %#v", rpc.Result)
	return result.ServerInfo.Icons
}
