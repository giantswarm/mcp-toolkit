package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// DefaultShutdownTimeout is the drain budget used when no timeout is supplied.
// Consumers must reserve time for other cleanup within their termination budget.
const DefaultShutdownTimeout = 5 * time.Second

// Run serves until ctx is canceled or the listener returns. Cancellation drains
// active requests within shutdownTimeout; zero uses DefaultShutdownTimeout.
// A drain timeout returns an error without closing active connections.
func Run(ctx context.Context, server *http.Server, shutdownTimeout time.Duration) error {
	return RunWithShutdownContext(ctx, server, shutdownTimeout, func() context.Context { return context.WithoutCancel(ctx) })
}

// RunWithShutdownContext serves like Run and bounds draining by both
// shutdownTimeout and the caller's shutdown context. The factory is invoked
// only after serving cancellation, so the application can start one process
// deadline then and share it with jobs, dependency cleanup and telemetry.
// The caller remains responsible for closing connections after a drain error.
func RunWithShutdownContext(ctx context.Context, server *http.Server, shutdownTimeout time.Duration, shutdownContext func() context.Context) error {
	if shutdownContext == nil {
		return errors.New("shutdown context factory is required")
	}
	if shutdownTimeout == 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}
	serverErrors := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErrors <- err
	}()
	select {
	case err := <-serverErrors:
		return err
	case <-ctx.Done():
	}
	drainContext, cancel := context.WithTimeout(shutdownContext(), shutdownTimeout)
	defer cancel()
	shutdownError := server.Shutdown(drainContext)
	startError := <-serverErrors
	if shutdownError != nil {
		return fmt.Errorf("shutdown: %w", shutdownError)
	}
	return startError
}
