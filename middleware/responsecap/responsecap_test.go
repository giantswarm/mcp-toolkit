package responsecap_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/middleware/responsecap"
)

func textHandler(text string) server.ToolHandlerFunc {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}},
		}, nil
	}
}

func req(name string) mcp.CallToolRequest {
	r := mcp.CallToolRequest{}
	r.Params.Name = name
	return r
}

func TestPassThroughWhenUnderLimit(t *testing.T) {
	mw := responsecap.New(responsecap.Options{Limit: 100})
	res, err := mw(textHandler("ok"))(context.Background(), req("any"))
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, "ok", res.Content[0].(mcp.TextContent).Text)
}

func TestReplacesOversizedTextContent(t *testing.T) {
	body := strings.Repeat("x", 200)
	mw := responsecap.New(responsecap.Options{Limit: 100})
	res, err := mw(textHandler(body))(context.Background(), req("any"))
	require.NoError(t, err)
	require.True(t, res.IsError)

	var e responsecap.Error
	require.NoError(t, json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &e))
	require.Equal(t, responsecap.ErrCode, e.Error)
	require.Equal(t, 200, e.Bytes)
	require.Equal(t, 100, e.Limit)
	require.NotEmpty(t, e.Hint)
}

func TestZeroLimitAppliesDefault(t *testing.T) {
	body := strings.Repeat("x", responsecap.DefaultLimit+1)
	mw := responsecap.New(responsecap.Options{}) // zero-value, safe by default
	res, err := mw(textHandler(body))(context.Background(), req("any"))
	require.NoError(t, err)
	require.True(t, res.IsError, "zero Limit must fall back to DefaultLimit")

	var e responsecap.Error
	require.NoError(t, json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &e))
	require.Equal(t, responsecap.DefaultLimit, e.Limit)
}

func TestDisabledWhenLimitNegative(t *testing.T) {
	body := strings.Repeat("x", responsecap.DefaultLimit+1)
	mw := responsecap.New(responsecap.Options{Limit: -1})
	res, err := mw(textHandler(body))(context.Background(), req("any"))
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, body, res.Content[0].(mcp.TextContent).Text)
}

func TestAllowOverridePredicate(t *testing.T) {
	body := strings.Repeat("x", 200)
	mw := responsecap.New(responsecap.Options{
		Limit:         100,
		AllowOverride: func(name string, _ mcp.CallToolRequest) bool { return name == "skip" },
	})

	res, err := mw(textHandler(body))(context.Background(), req("skip"))
	require.NoError(t, err)
	require.False(t, res.IsError)

	res, err = mw(textHandler(body))(context.Background(), req("other"))
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestPerToolHint(t *testing.T) {
	body := strings.Repeat("x", 200)
	mw := responsecap.New(responsecap.Options{
		Limit: 100,
		Hint:  func(name string) string { return "narrow " + name },
	})
	res, err := mw(textHandler(body))(context.Background(), req("foo"))
	require.NoError(t, err)

	var e responsecap.Error
	require.NoError(t, json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &e))
	require.Equal(t, "narrow foo", e.Hint)
}

func TestNonTextContentUntouched(t *testing.T) {
	mw := responsecap.New(responsecap.Options{Limit: 1})
	handler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.ImageContent{Type: "image", Data: strings.Repeat("Z", 200), MIMEType: "image/png"},
			},
		}, nil
	}
	res, err := mw(handler)(context.Background(), req("any"))
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.IsType(t, mcp.ImageContent{}, res.Content[0])
}

func TestPropagatesHandlerError(t *testing.T) {
	handler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, errors.New("boom")
	}
	mw := responsecap.New(responsecap.Options{Limit: 100})
	res, err := mw(handler)(context.Background(), req("any"))
	require.Error(t, err)
	require.Nil(t, res)
}

func TestNilResultPassThrough(t *testing.T) {
	handler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, nil
	}
	mw := responsecap.New(responsecap.Options{Limit: 100})
	res, err := mw(handler)(context.Background(), req("any"))
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestMultipleContentEvaluatedIndependently(t *testing.T) {
	mw := responsecap.New(responsecap.Options{Limit: 5})
	handler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "ok"},
				mcp.TextContent{Type: "text", Text: strings.Repeat("x", 200)},
				mcp.TextContent{Type: "text", Text: "fine"},
			},
		}, nil
	}
	res, err := mw(handler)(context.Background(), req("any"))
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Equal(t, "ok", res.Content[0].(mcp.TextContent).Text)
	require.Contains(t, res.Content[1].(mcp.TextContent).Text, responsecap.ErrCode)
	require.Equal(t, "fine", res.Content[2].(mcp.TextContent).Text)
}

func TestExemptExemptsTools(t *testing.T) {
	body := strings.Repeat("x", 200)
	mw := responsecap.New(responsecap.Options{
		Limit:  100,
		Exempt: func(name string) bool { return name == "check_ready" },
	})

	res, err := mw(textHandler(body))(context.Background(), req("check_ready"))
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Equal(t, body, res.Content[0].(mcp.TextContent).Text)

	res, err = mw(textHandler(body))(context.Background(), req("other"))
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestExemptShortCircuitsAllowOverride(t *testing.T) {
	body := strings.Repeat("x", 200)
	allowOverrideCalled := false
	mw := responsecap.New(responsecap.Options{
		Limit:  100,
		Exempt: func(string) bool { return true },
		AllowOverride: func(string, mcp.CallToolRequest) bool {
			allowOverrideCalled = true
			return false
		},
	})
	res, err := mw(textHandler(body))(context.Background(), req("any"))
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.False(t, allowOverrideCalled, "Exempt should short-circuit before AllowOverride")
}

func TestAllowOverrideReceivesRequest(t *testing.T) {
	body := strings.Repeat("x", 200)
	var seen string
	mw := responsecap.New(responsecap.Options{
		Limit: 100,
		AllowOverride: func(name string, _ mcp.CallToolRequest) bool {
			seen = name
			return false
		},
	})
	_, err := mw(textHandler(body))(context.Background(), req("tool-x"))
	require.NoError(t, err)
	require.Equal(t, "tool-x", seen)
}

func TestNilContentPassThrough(t *testing.T) {
	handler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: nil}, nil
	}
	mw := responsecap.New(responsecap.Options{Limit: 100})
	res, err := mw(handler)(context.Background(), req("any"))
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Nil(t, res.Content)
}
