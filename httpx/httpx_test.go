package httpx_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/httpx"
)

func startServer(t *testing.T, handler http.Handler, run func(context.Context, *http.Server) error) (*http.Server, string, context.CancelFunc, <-chan error) {
	t.Helper()
	addresses := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.Handle("/", handler)
	server := &http.Server{
		Addr: "127.0.0.1:0", Handler: mux, ReadHeaderTimeout: time.Second,
		BaseContext: func(listener net.Listener) context.Context {
			addresses <- listener.Addr().String()
			return t.Context()
		},
	}
	servingContext, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- run(servingContext, server) }()
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
	})
	address := receive(t, addresses)
	client := &http.Client{Timeout: 100 * time.Millisecond}
	require.Eventually(t, func() bool {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/ping", nil)
		if err != nil {
			return false
		}
		response, err := client.Do(request)
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode == http.StatusNoContent
	}, 3*time.Second, 10*time.Millisecond)
	return server, address, cancel, done
}

func receive[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not finish")
		var zero T
		return zero
	}
}

func TestRunDrainsRequests(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			server, address, cancel, done := startServer(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				close(entered)
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
				_, _ = io.WriteString(w, "complete")
			}), func(ctx context.Context, server *http.Server) error { return httpx.Run(ctx, server, timeout) })
			responseBody := make(chan string, 1)
			go func() {
				request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/work", nil)
				if err != nil {
					responseBody <- err.Error()
					return
				}
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					responseBody <- err.Error()
					return
				}
				defer func() { _ = response.Body.Close() }()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					responseBody <- err.Error()
					return
				}
				responseBody <- string(body)
			}()
			receive(t, entered)
			shuttingDown := make(chan struct{})
			server.RegisterOnShutdown(func() { close(shuttingDown) })
			cancel()
			receive(t, shuttingDown)
			select {
			case err := <-done:
				t.Fatalf("returned before request completed: %v", err)
			default:
			}
			// The request owns its completion independently of the serving context.
			release <- struct{}{}
			require.Equal(t, "complete", receive(t, responseBody))
			require.NoError(t, receive(t, done))
		})
	}
}

func TestRunSurfacesBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	server := &http.Server{Addr: listener.Addr().String(), ReadHeaderTimeout: time.Second}
	err = httpx.Run(t.Context(), server, time.Second)
	require.Error(t, err)
	require.NotErrorIs(t, err, http.ErrServerClosed)
}

func TestRunTimeoutLeavesConnectionsOpen(t *testing.T) {
	entered := make(chan struct{})
	disconnected := make(chan struct{})
	server, address, cancel, done := startServer(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(entered)
		<-request.Context().Done()
		close(disconnected)
	}), func(ctx context.Context, server *http.Server) error {
		return httpx.Run(ctx, server, 25*time.Millisecond)
	})
	clientDone := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/work", nil)
		if err != nil {
			clientDone <- err
			return
		}
		response, err := http.DefaultClient.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		clientDone <- err
	}()
	receive(t, entered)
	cancel()
	require.ErrorIs(t, receive(t, done), context.DeadlineExceeded)
	select {
	case <-disconnected:
		t.Fatal("legacy policy closed the active request")
	default:
	}
	require.NoError(t, server.Close())
	receive(t, disconnected)
	require.Error(t, receive(t, clientDone))
}

func TestRunSharesProcessDeadline(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var calls atomic.Int32
	var process context.Context
	server, address, cancel, done := startServer(t, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-request.Context().Done():
		}
	}), func(ctx context.Context, server *http.Server) error {
		return httpx.RunWithShutdownContext(ctx, server, time.Minute, func() context.Context {
			calls.Add(1)
			var cancel context.CancelFunc
			process, cancel = context.WithTimeout(t.Context(), 25*time.Millisecond)
			t.Cleanup(cancel)
			return process
		})
	})
	require.Zero(t, calls.Load())
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/work", nil)
		if err != nil {
			return
		}
		response, _ := http.DefaultClient.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
	}()
	receive(t, entered)
	cancel()
	require.ErrorIs(t, receive(t, done), context.DeadlineExceeded)
	require.Equal(t, int32(1), calls.Load())
	require.ErrorIs(t, process.Err(), context.DeadlineExceeded)
	require.NoError(t, server.Close())
	receive(t, clientDone)
}

func TestRunShutdownFactoryValidationAndBindFailure(t *testing.T) {
	require.Error(t, httpx.RunWithShutdownContext(t.Context(), &http.Server{ReadHeaderTimeout: time.Second}, time.Second, nil))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, listener.Close()) }()
	calls := 0
	err = httpx.RunWithShutdownContext(t.Context(), &http.Server{Addr: listener.Addr().String(), ReadHeaderTimeout: time.Second}, time.Second, func() context.Context { calls++; return t.Context() })
	require.Error(t, err)
	require.Zero(t, calls)
}
