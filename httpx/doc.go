// Package httpx provides a small graceful-shutdown wrapper around
// net/http for Giant Swarm MCP servers.
//
// Run starts srv.ListenAndServe in a goroutine and blocks until
// either the parent context is canceled (in which case it calls
// srv.Shutdown) or the server itself returns an error.
//
// For TLS or custom listeners, configure srv.TLSConfig or call
// srv.Serve(listener) yourself in a small wrapper — this package
// deliberately does not abstract listener choice.
package httpx
