package logging

import (
	"context"
	"errors"
	"log/slog"
)

// fanoutHandler dispatches every slog.Record to all wrapped handlers.
// Enabled is the OR of children — any non-rejecting child keeps the
// record alive; Handle calls each child whose Enabled returns true
// for the record's level and joins their errors.
type fanoutHandler struct {
	handlers []slog.Handler
}

// newFanout returns a slog.Handler that mirrors every record to all
// of handlers. A single-element slice is returned as-is so the common
// "no extras" path adds no indirection.
func newFanout(handlers []slog.Handler) slog.Handler {
	if len(handlers) == 1 {
		return handlers[0]
	}
	return fanoutHandler{handlers: handlers}
}

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, sh := range h.handlers {
		if sh.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, sh := range h.handlers {
		if !sh.Enabled(ctx, r.Level) {
			continue
		}
		// Each child gets its own clone — slog.Handler implementations
		// may add attributes during Handle, and a shared Record would
		// let one child see another's additions.
		if err := sh.Handle(ctx, r.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(h.handlers))
	for i, sh := range h.handlers {
		out[i] = sh.WithAttrs(attrs)
	}
	return fanoutHandler{handlers: out}
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(h.handlers))
	for i, sh := range h.handlers {
		out[i] = sh.WithGroup(name)
	}
	return fanoutHandler{handlers: out}
}
