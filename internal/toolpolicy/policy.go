package toolpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/semconv/v1.41.0/mcpconv"

	"github.com/giantswarm/mcp-toolkit/toolpolicy"
)

type Outcome string

const (
	Success       Outcome = "success"
	ToolError     Outcome = "tool_error"
	ProtocolError Outcome = "protocol_error"
	Cancelled     Outcome = "cancelled"
	Timeout       Outcome = "timeout"
)

var errToolDeadline = errors.New("tool deadline exceeded")

var ErrInternal = errors.New("internal tool error")

type Policy struct {
	options  toolpolicy.Options
	duration mcpconv.ServerOperationDuration
}

func New(options toolpolicy.Options) *Policy {
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Second
	}
	if options.Meter == nil {
		options.Meter = otel.Meter("github.com/giantswarm/mcp-toolkit")
	}
	duration, err := mcpconv.NewServerOperationDuration(options.Meter)
	if err != nil {
		otel.Handle(err)
	}
	return &Policy{options: options, duration: duration}
}

func (p *Policy) Context(parent context.Context) (context.Context, context.CancelFunc) {
	if p.options.Timeout < 0 {
		return parent, func() {}
	}
	return context.WithTimeoutCause(parent, p.options.Timeout, errToolDeadline)
}

func (p *Policy) TimedOut(parent, call context.Context) bool {
	return parent.Err() == nil && errors.Is(context.Cause(call), errToolDeadline)
}

func (p *Policy) TimeoutMessage(tool string) string {
	return fmt.Sprintf("tool %q exceeded timeout of %s", tool, p.options.Timeout)
}

func (p *Policy) ResultTooLarge(result any) bool {
	if p.options.MaxResultBytes <= 0 {
		return false
	}
	encoded, err := json.Marshal(result)
	return err == nil && len(encoded) > p.options.MaxResultBytes
}

func (p *Policy) Outcome(parent, call context.Context, handlerError error, toolError bool) Outcome {
	switch {
	case parent.Err() != nil:
		return Cancelled
	case p.TimedOut(parent, call):
		return Timeout
	case handlerError != nil:
		return ProtocolError
	case toolError:
		return ToolError
	default:
		return Success
	}
}

func (p *Policy) Record(ctx context.Context, tool string, started time.Time, outcome Outcome) {
	attributes := []attribute.KeyValue{
		p.duration.AttrGenAIToolName(tool),
		p.duration.AttrGenAIOperationName(mcpconv.GenAIOperationNameExecuteTool),
	}
	if outcome != Success {
		errorType := mcpconv.ErrorTypeAttr(outcome)
		if outcome == ProtocolError {
			errorType = mcpconv.ErrorTypeOther
		}
		attributes = append(attributes, p.duration.AttrErrorType(errorType))
	}
	p.duration.Record(ctx, time.Since(started).Seconds(), mcpconv.MethodNameToolsCall, attributes...)
	if p.options.Logger == nil {
		return
	}
	fields := []slog.Attr{slog.String("tool", tool), slog.String("outcome", string(outcome))}
	if p.options.User != nil {
		if user := p.options.User(ctx); user != nil {
			fields = append(fields, slog.Any("user", user))
		}
	}
	p.options.Logger.LogAttrs(ctx, slog.LevelInfo, "tool_call", fields...)
}
