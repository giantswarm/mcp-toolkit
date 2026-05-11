package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewStderrMirrorHandler_HonoursLevel(t *testing.T) {
	cases := []struct {
		name         string
		handlerLevel slog.Level
		recordLevel  slog.Level
		wantEmitted  bool
	}{
		{"info passes when handler is info", slog.LevelInfo, slog.LevelInfo, true},
		{"debug dropped when handler is info", slog.LevelInfo, slog.LevelDebug, false},
		{"warn passes when handler is info", slog.LevelInfo, slog.LevelWarn, true},
		{"info dropped when handler is warn", slog.LevelWarn, slog.LevelInfo, false},
		{"debug passes when handler is debug", slog.LevelDebug, slog.LevelDebug, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := newStderrMirrorHandler(&buf, tc.handlerLevel)
			slog.New(h).Log(context.Background(), tc.recordLevel, "test")
			if tc.wantEmitted {
				require.NotEmpty(t, buf.Bytes(), "expected record to be emitted")
				var rec map[string]any
				require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
				require.Equal(t, "test", rec["msg"])
			} else {
				require.Empty(t, buf.Bytes(), "expected record to be dropped by level filter")
			}
		})
	}
}
