package gosdk_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	legacy "github.com/giantswarm/mcp-toolkit/brand"
	"github.com/giantswarm/mcp-toolkit/brand/gosdk"
)

func TestNativeIconPreservesWireRepresentation(t *testing.T) {
	expected, err := json.Marshal(legacy.GiantSwarmIcon())
	require.NoError(t, err)
	actual, err := json.Marshal(gosdk.GiantSwarmIcon())
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(actual))
}
