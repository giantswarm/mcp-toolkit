package health_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/health"
)

func TestNew_StartsNotReady(t *testing.T) {
	h := health.New()
	require.False(t, h.IsReady(), "fresh Health must start not-ready so kubelet does not route traffic before initialization completes")
}

func TestSetReady_TogglesState(t *testing.T) {
	h := health.New()
	h.SetReady(true)
	require.True(t, h.IsReady())
	h.SetReady(false)
	require.False(t, h.IsReady())
}

func TestLiveness_AlwaysReturns200(t *testing.T) {
	h := health.New()
	// Even when not ready, liveness must succeed: a flaky liveness
	// would have kubelet restart the pod for no reason.
	for _, ready := range []bool{false, true} {
		h.SetReady(ready)
		rec := httptest.NewRecorder()
		h.Liveness(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		require.Equal(t, http.StatusOK, rec.Code, "liveness must be 200 when ready=%v", ready)
		require.Equal(t, "ok", rec.Body.String())
	}
}

func TestReadiness_503WhenNotReady(t *testing.T) {
	h := health.New()
	rec := httptest.NewRecorder()
	h.Readiness(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not ready", rec.Body.String())
}

func TestReadiness_200WhenReady(t *testing.T) {
	h := health.New()
	h.SetReady(true)
	rec := httptest.NewRecorder()
	h.Readiness(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ready", rec.Body.String())
}

func TestMount_RegistersBothEndpoints(t *testing.T) {
	h := health.New()
	h.SetReady(true)
	mux := http.NewServeMux()
	h.Mount(mux)

	for path, wantStatus := range map[string]int{
		"/healthz": http.StatusOK,
		"/readyz":  http.StatusOK,
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, wantStatus, rec.Code, "path %s", path)
	}
}

func TestSetReady_ConcurrentSafe(t *testing.T) {
	h := health.New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); h.SetReady(true) }()
		go func() { defer wg.Done(); _ = h.IsReady() }()
	}
	wg.Wait()
}
