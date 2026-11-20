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

// RunOptions configures HTTP draining.
type RunOptions struct {
	// ShutdownTimeout bounds graceful draining. Zero uses DefaultShutdownTimeout.
	ShutdownTimeout time.Duration
	// ForceCloseOnTimeout closes remaining ordinary HTTP connections when the
	// drain deadline expires. It does not close hijacked connections or stop
	// handlers that ignore request cancellation.
	ForceCloseOnTimeout bool
}

// Run serves until ctx is canceled or the listener returns. Cancellation drains
// active requests within shutdownTimeout; zero uses DefaultShutdownTimeout.
// A drain timeout returns an error without closing active connections.
func Run(ctx context.Context, server *http.Server, shutdownTimeout time.Duration) error {
	return RunWithOptions(ctx, server, RunOptions{ShutdownTimeout: shutdownTimeout})
}

// RunWithOptions serves plain HTTP and drains on cancellation. ErrServerClosed
// is treated as a clean exit. Callers own readiness and dependent resources:
// revoke readiness before canceling ctx and keep dependencies alive until drain
// completes. TLS and custom listeners require their own serving implementation.
func RunWithOptions(ctx context.Context, server *http.Server, options RunOptions) error {
	if options.ShutdownTimeout == 0 {
		options.ShutdownTimeout = DefaultShutdownTimeout
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
	shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), options.ShutdownTimeout)
	defer cancel()
	shutdownError := server.Shutdown(shutdownContext)
	var closeError error
	if options.ForceCloseOnTimeout && errors.Is(shutdownError, context.DeadlineExceeded) {
		closeError = server.Close()
	}
	startError := <-serverErrors
	if shutdownError != nil {
		return errors.Join(fmt.Errorf("shutdown: %w", shutdownError), closeError, startError)
	}
	return startError
}
