package health

import (
	"net/http"
	"sync/atomic"
)

// Health serves GET and HEAD on /healthz (200) and /readyz (200 if the
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

// Mount reserves /healthz and /readyz on mux for all methods.
func (h *Health) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.Liveness)
	mux.HandleFunc("/readyz", h.Readiness)
}

// Liveness returns 200 for GET and HEAD, or 405 for unsupported methods.
func (h *Health) Liveness(w http.ResponseWriter, request *http.Request) {
	if !allowProbeMethod(w, request) {
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Readiness returns 200 when ready, 503 when not, or 405 for unsupported methods.
func (h *Health) Readiness(w http.ResponseWriter, request *http.Request) {
	if !allowProbeMethod(w, request) {
		return
	}
	if h.ready.Load() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("not ready"))
}

func allowProbeMethod(w http.ResponseWriter, request *http.Request) bool {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return false
	}
	return true
}
