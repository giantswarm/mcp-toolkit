package health

import (
	"net/http"
	"sync/atomic"
)

// Health serves /healthz (unconditional 200) and /readyz (200 if the
// server has called SetReady(true) and not since revoked it; 503
// otherwise).
//
// The zero value is not usable; callers must use New.
type Health struct {
	ready atomic.Bool
}

// New returns a Health that starts not-ready. Servers should call
// SetReady(true) once initialization completes and SetReady(false)
// when graceful shutdown begins.
func New() *Health {
	return &Health{}
}

// SetReady toggles the readiness state.
func (h *Health) SetReady(ready bool) {
	h.ready.Store(ready)
}

// IsReady reports the current readiness state.
func (h *Health) IsReady() bool {
	return h.ready.Load()
}

// Mount registers /healthz and /readyz on mux.
func (h *Health) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.Liveness)
	mux.HandleFunc("/readyz", h.Readiness)
}

// Liveness is the /healthz handler: always 200.
func (h *Health) Liveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Readiness is the /readyz handler: 200 when ready, 503 when not.
func (h *Health) Readiness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if h.ready.Load() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("not ready"))
}
