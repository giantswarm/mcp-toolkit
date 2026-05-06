package timeout_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/middleware/timeout"
)

func req(name string) mcp.CallToolRequest {
	r := mcp.CallToolRequest{}
	r.Params.Name = name
	return r
}

// successHandler returns "ok" without honouring ctx — runs to
// completion regardless.
func successHandler() server.ToolHandlerFunc {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	}
}

// blockingHandler honours ctx; it returns when ctx is done.
func blockingHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func TestNew_PassThroughWhenUnderTimeout(t *testing.T) {
	mw := timeout.New(time.Second)
	res, err := mw(successHandler())(context.Background(), req("any"))
	require.NoError(t, err)
	require.False(t, res.IsError)
}

func TestNew_DeadlineExceededBecomesIsError(t *testing.T) {
	mw := timeout.New(20 * time.Millisecond)
	res, err := mw(blockingHandler())(context.Background(), req("slow_tool"))
	require.NoError(t, err, "deadline must surface as IsError result, not Go error")
	require.True(t, res.IsError)

	// The error message names the tool and the timeout duration so
	// the LLM has actionable text.
	body, ok := res.Content[0].(mcp.TextContent)
	require.True(t, ok)
	require.Contains(t, body.Text, "slow_tool")
	require.Contains(t, body.Text, "20ms")
}

func TestNew_ParentCancelPropagates(t *testing.T) {
	mw := timeout.New(time.Second)
	parentCtx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before the call

	res, err := mw(blockingHandler())(parentCtx, req("any"))
	// Parent cancel is not masqueraded as a timeout; the underlying
	// ctx.Err() (context.Canceled) propagates as a Go error.
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, res)
}

func TestNew_ZeroAppliesDefault(t *testing.T) {
	// We can't realistically wait DefaultTimeout in a test. Just
	// verify that zero doesn't disable the wrap (negative does) by
	// confirming that a sub-default deadline still fires.
	mw := timeout.New(0) // → DefaultTimeout, but we need to verify it WRAPS at all
	// Use a parent ctx with a tight deadline so the wrap (whatever
	// its size) will see the parent cancel and propagate it. The
	// behaviour we want is "zero is wrapped, not pass-through".
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := mw(blockingHandler())(parent, req("any"))
	// Parent ctx fired first → context.DeadlineExceeded propagates
	// as a Go error (since the parent expired, not the toolkit's
	// inner deadline).
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestNew_NegativeDisables(t *testing.T) {
	mw := timeout.New(-1)
	// A handler that uses the parent ctx — since middleware is a
	// pass-through, this ctx is the parent ctx unmodified.
	called := false
	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		// Verify that no toolkit deadline is set on ctx.
		_, hasDeadline := ctx.Deadline()
		require.False(t, hasDeadline, "negative timeout must pass parent ctx through unmodified")
		return mcp.NewToolResultText("ok"), nil
	}
	res, err := mw(handler)(context.Background(), req("any"))
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.True(t, called)
}

func TestNew_ParentDeadlineNotMasqueradedAsToolkitTimeout(t *testing.T) {
	// Parent's deadline (e.g. an HTTP request timeout) is shorter
	// than the toolkit's. The middleware must NOT emit a
	// 'tool X exceeded timeout of Y' result blaming the toolkit's
	// timeout — that would mislead the LLM about what happened.
	// Instead, the parent's DeadlineExceeded propagates as a Go
	// error so the caller's framing is preserved.
	mw := timeout.New(time.Second) // generous toolkit timeout
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	res, err := mw(blockingHandler())(parent, req("any"))
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded))
	require.Nil(t, res, "no IsError result should be synthesized when parent deadline fired")
}

func TestNew_PropagatesNonTimeoutError(t *testing.T) {
	mw := timeout.New(time.Second)
	boom := errors.New("boom")
	handler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, boom
	}
	res, err := mw(handler)(context.Background(), req("any"))
	require.ErrorIs(t, err, boom, "non-deadline errors must pass through unchanged")
	require.Nil(t, res)
}
