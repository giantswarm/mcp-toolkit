package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFanout_DispatchesToAllHandlers(t *testing.T) {
	var a, b bytes.Buffer
	h := newFanout([]slog.Handler{
		slog.NewJSONHandler(&a, nil),
		slog.NewJSONHandler(&b, nil),
	})
	slog.New(h).Info("hello", "k", "v")

	for name, buf := range map[string]*bytes.Buffer{"a": &a, "b": &b} {
		var rec map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &rec), "handler %s should have produced JSON", name)
		require.Equal(t, "hello", rec["msg"])
		require.Equal(t, "v", rec["k"])
	}
}

func TestFanout_EnabledIsOR(t *testing.T) {
	infoBuf := &bytes.Buffer{}
	warnBuf := &bytes.Buffer{}
	h := newFanout([]slog.Handler{
		slog.NewJSONHandler(infoBuf, &slog.HandlerOptions{Level: slog.LevelInfo}),
		slog.NewJSONHandler(warnBuf, &slog.HandlerOptions{Level: slog.LevelWarn}),
	})
	require.True(t, h.Enabled(context.Background(), slog.LevelInfo), "info enabled because the info handler accepts it")
	require.True(t, h.Enabled(context.Background(), slog.LevelWarn), "warn enabled by both handlers")
	require.False(t, h.Enabled(context.Background(), slog.LevelDebug), "debug rejected by both")
}

func TestFanout_PerHandlerLevelFilter(t *testing.T) {
	var infoBuf, warnBuf bytes.Buffer
	h := newFanout([]slog.Handler{
		slog.NewJSONHandler(&infoBuf, &slog.HandlerOptions{Level: slog.LevelInfo}),
		slog.NewJSONHandler(&warnBuf, &slog.HandlerOptions{Level: slog.LevelWarn}),
	})
	l := slog.New(h)
	l.Info("info-line")
	l.Warn("warn-line")

	require.Contains(t, infoBuf.String(), `"msg":"info-line"`)
	require.Contains(t, infoBuf.String(), `"msg":"warn-line"`)
	require.NotContains(t, warnBuf.String(), `"msg":"info-line"`, "info-level record filtered out by warn-only handler")
	require.Contains(t, warnBuf.String(), `"msg":"warn-line"`)
}

func TestFanout_SingleHandlerReturnedAsIs(t *testing.T) {
	inner := slog.NewJSONHandler(&bytes.Buffer{}, nil)
	out := newFanout([]slog.Handler{inner})
	require.Equal(t, inner, out, "single-element slice should be returned unwrapped — no fan-out overhead")
}

func TestFanout_WithAttrs_PropagatesToEveryHandler(t *testing.T) {
	var a, b bytes.Buffer
	h := newFanout([]slog.Handler{
		slog.NewJSONHandler(&a, nil),
		slog.NewJSONHandler(&b, nil),
	})
	l := slog.New(h).With("service", "test")
	l.Info("hello")

	for name, buf := range map[string]*bytes.Buffer{"a": &a, "b": &b} {
		var rec map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &rec), "handler %s", name)
		require.Equal(t, "test", rec["service"], "WithAttrs must propagate to every child")
	}
}
