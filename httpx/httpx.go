package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// DefaultShutdownTimeout is the value applied when Run is given a
// zero shutdownTimeout. Long enough for in-flight tool calls to drain;
// short enough that kubelet's terminationGracePeriodSeconds (default
// 30s) has slack for SIGKILL fallback.
const DefaultShutdownTimeout = 5 * time.Second

// Run starts srv.ListenAndServe and blocks until either ctx is
// canceled or the server returns. On ctx cancel it calls srv.Shutdown
// with the given timeout (DefaultShutdownTimeout when zero) and
// returns nil for clean shutdown.
//
// http.ErrServerClosed is filtered out — callers see a nil error on
// graceful shutdown, and the actual underlying error otherwise.
//
// For TLS, configure srv.TLSConfig and call srv.ListenAndServeTLS
// in your own wrapper. This package does not abstract listener
// choice.
func Run(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration) error {
	if shutdownTimeout == 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}

	serverErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErr <- err
	}()

	select {
	case err := <-serverErr:
		// Server returned before ctx was canceled — typically a bind
		// failure (port in use, missing permission). Surface as-is.
		return err
	case <-ctx.Done():
		// Use a fresh context for Shutdown — ctx is already done, so
		// inheriting from it would cause Shutdown to bail immediately.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		shutdownErr := srv.Shutdown(shutdownCtx)
		// Wait for ListenAndServe to actually return (it does so after
		// Shutdown finishes draining or the deadline fires).
		startErr := <-serverErr
		if shutdownErr != nil {
			return fmt.Errorf("shutdown: %w", shutdownErr)
		}
		return startErr
	}
}
