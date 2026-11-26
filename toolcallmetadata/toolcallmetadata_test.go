package toolcallmetadata_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/toolcallmetadata"
)

func TestValuesPreservesApplicationMetadata(t *testing.T) {
	metadata := map[string]any{
		"io.giantswarm/tool-call-metadata": map[string]any{"session": "a1", "any field": true},
		"com.example/request-id":           "request-1",
		"plain":                            "value",
	}
	values := toolcallmetadata.Values(metadata)
	require.Equal(t, metadata, values)
	values["plain"] = "changed"
	require.Equal(t, "value", metadata["plain"])
	require.Nil(t, toolcallmetadata.Values(nil))
}

func TestString(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		metadata map[string]any
		value    string
		present  bool
	}{
		{name: "string", metadata: map[string]any{"session": "a1"}, value: "a1", present: true},
		{name: "empty string", metadata: map[string]any{"session": ""}, present: true},
		{name: "number", metadata: map[string]any{"session": 3.0}},
		{name: "missing", metadata: map[string]any{}},
		{name: "no metadata"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			value, present := toolcallmetadata.String(scenario.metadata, "session")
			require.Equal(t, scenario.value, value)
			require.Equal(t, scenario.present, present)
		})
	}
}
