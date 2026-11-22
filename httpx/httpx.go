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
	// The serving context is canceled, so draining needs an independent deadline.
	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	shutdownError := server.Shutdown(shutdownContext)
	startError := <-serverErrors
	if shutdownError != nil {
		return fmt.Errorf("shutdown: %w", shutdownError)
	}
	return startError
}
