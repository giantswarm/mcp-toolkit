package httpx_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/httpx"
)

func TestRun_ReturnsCleanlyOnContextCancel(t *testing.T) {
	srv := &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		ReadHeaderTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- httpx.Run(ctx, srv, time.Second) }()

	// Give the goroutine a moment to call ListenAndServe.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err, "graceful shutdown must surface as nil; ErrServerClosed must be filtered")
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of context cancel")
	}
}

func TestRun_SurfacesBindFailure(t *testing.T) {
	// Pre-bind a port to force ListenAndServe to fail.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	srv := &http.Server{
		Addr:              ln.Addr().String(),
		ReadHeaderTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = httpx.Run(ctx, srv, time.Second)
	require.Error(t, err, "bind failure must be surfaced")
	require.NotErrorIs(t, err, http.ErrServerClosed)
}

func TestRun_ShutdownTimeoutAppliesWhenZero(t *testing.T) {
	// Sanity check the default-applies branch: the test passes if Run
	// returns within DefaultShutdownTimeout + a small margin after
	// ctx cancel, not the (much shorter) explicit zero we passed.
	srv := &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		ReadHeaderTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- httpx.Run(ctx, srv, 0) }()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(httpx.DefaultShutdownTimeout + time.Second):
		t.Fatalf("Run did not return within DefaultShutdownTimeout (%s) + 1s margin", httpx.DefaultShutdownTimeout)
	}
}

func TestRun_ShutdownDeadlineExceededIsSurfaced(t *testing.T) {
	// Handler that ignores its context deadline. With a tight
	// shutdown timeout, Shutdown must return context.DeadlineExceeded,
	// and Run must wrap and surface it.
	blocked := make(chan struct{})
	defer close(blocked)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           handler,
		ReadHeaderTimeout: time.Second,
	}
	require.NoError(t, ln.Close()) // free the port; Run will rebind.

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- httpx.Run(ctx, srv, 50*time.Millisecond) }()

	// Issue a request that will hang in the handler so Shutdown has
	// something to wait on.
	time.Sleep(50 * time.Millisecond)
	go func() {
		req, _ := http.NewRequest(http.MethodGet, "http://"+srv.Addr+"/", nil)
		_, _ = http.DefaultClient.Do(req)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.Error(t, err, "Shutdown must surface deadline-exceeded when probes are stuck")
		require.Contains(t, err.Error(), "shutdown")
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of ctx cancel")
	}
}
