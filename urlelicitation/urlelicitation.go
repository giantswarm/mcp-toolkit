// Package urlelicitation asks a tool's caller to open a URL, such as a
// third-party sign-in, with MCP URL mode elicitation, and passes on such a
// request from a server the tool called. It works with mcp-go servers.
//
// [Elicitor.Elicit] answers each caller the way its protocol revision
// defines:
//
//   - 2026-07-28 with elicitation.url declared: an InputRequiredResult with
//     the elicitation and a sealed requestState.
//   - 2025-11-25 with elicitation.url declared: URLElicitationRequiredError
//     (-32042) with a random elicitationId. No completion notification is
//     sent (the specification makes it optional), so the caller retries when
//     the person says they are done, and the retry runs as a new call.
//   - Otherwise: a tool error with the message and the URL, and the URL as a
//     resource_link from 2025-06-18.
//
// [Elicitor.Result] reads a 2026-07-28 retry. requestState is sealed by a
// [Sealer], bound to the validated caller, to the originating request
// (method, tool name and arguments) and to a TTL, so a caller cannot change
// it or present it for another person or request. Within the TTL the same
// person can present it again on the same request; a tool that must act at
// most once enforces that itself. The Sealer decides what of the caller it
// binds: mcp-oauth's requeststate.Sealer implements Sealer[*providers.UserInfo]
// and binds the issuer, subject, OAuth client and current actor:
//
//	sealer, err := oauthconfig.NewRequestStateSealerFromEnv()
//	...
//	elicitor, err := urlelicitation.New(sealer, urlelicitation.WithExpired(func(err error) bool {
//		return errors.Is(err, requeststate.ErrExpired)
//	}))
//	...
//	user, _ := handler.UserInfoFromContext(ctx)
//	res, err := elicitor.Elicit(ctx, req, user, params, state)
//
// The package does not make the URL safe on its own. The server behind the
// URL binds its flow to the person the elicitation was issued to and checks
// that the person who opens the URL is that person. An accept answer means
// the person agreed to open the URL, not that they finished: the tool checks
// its own store and elicits again when the flow has not completed. URL mode
// elicitation is for third-party authorization only, never for signing a
// person in to the MCP server itself.
package urlelicitation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Sealer seals state for a validated user of type U and an originating
// request, and opens it again for the same user and request.
type Sealer[U any] interface {
	Seal(user U, request string, payload []byte, ttl time.Duration) (string, error)
	Open(user U, request, sealed string) ([]byte, error)
}

// URLParams is one URL the caller is asked to open: the specification's
// ElicitRequestURLParams and the inputRequests key it is sent under.
type URLParams struct {
	// Key names the request in inputRequests.
	Key string
	// Message tells the person why the interaction is needed, naming the
	// server that asks.
	Message string
	// URL is the https URL to open. It must not be pre-authenticated.
	URL string
}

var (
	// ErrInvalidRequestState is a requestState that fails verification: not
	// sealed by this server, changed, or issued to another person or request.
	ErrInvalidRequestState = errors.New("urlelicitation: invalid requestState")
	// ErrInvalidResponse is an input response that is not an elicitation
	// result with a known action.
	ErrInvalidResponse = errors.New("urlelicitation: invalid input response")
)

// DefaultTTL is how long a caller has to answer.
const DefaultTTL = 15 * time.Minute

// Elicitor asks callers, validated users of type U, to open URLs and reads
// their results.
type Elicitor[U any] struct {
	sealer Sealer[U]
	config
}

type config struct {
	ttl     time.Duration
	expired func(error) bool
}

// Option configures an [Elicitor].
type Option func(*config)

// WithTTL sets how long a caller has to answer. A nonpositive ttl keeps
// [DefaultTTL].
func WithTTL(ttl time.Duration) Option {
	return func(c *config) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

// WithExpired tells which [Sealer.Open] errors mean the state lapsed. A retry
// with lapsed state is read as a first call, so the tool elicits again;
// without this option it is rejected as [ErrInvalidRequestState].
func WithExpired(expired func(error) bool) Option {
	return func(c *config) { c.expired = expired }
}

// New returns an Elicitor that seals with sealer.
func New[U any](sealer Sealer[U], opts ...Option) (*Elicitor[U], error) {
	if sealer == nil {
		return nil, errors.New("urlelicitation: a sealer is required")
	}
	e := &Elicitor[U]{sealer: sealer, config: config{ttl: DefaultTTL, expired: func(error) bool { return false }}}
	for _, opt := range opts {
		opt(&e.config)
	}
	return e, nil
}

// sealedState is what requestState carries: the inputRequests key the
// result comes back under and the tool's own state.
type sealedState struct {
	Key   string `json:"k"`
	State []byte `json:"s,omitempty"`
}

// Elicit answers the call with p, carrying state back on a 2026-07-28 retry.
// See the package documentation for the answer per revision. A returned
// error is meant to be returned by the tool handler as is.
func (e *Elicitor[U]) Elicit(ctx context.Context, req mcp.CallToolRequest, who U, p URLParams, state []byte) (*mcp.CallToolResult, error) {
	if p.Key == RelayKey {
		return nil, fmt.Errorf("urlelicitation: the key %q is reserved for Relay", RelayKey)
	}
	return e.elicit(ctx, req, who, p, state)
}

func (e *Elicitor[U]) elicit(ctx context.Context, req mcp.CallToolRequest, who U, p URLParams, state []byte) (*mcp.CallToolResult, error) {
	if err := validParams(p); err != nil {
		return nil, err
	}
	if !SupportsURL(ctx) {
		return fallback(ctx, p), nil
	}
	params := mcp.ElicitationParams{Mode: mcp.ElicitationModeURL, Message: p.Message, URL: p.URL}
	if !server.IsModernRequest(ctx) {
		params.ElicitationID = rand.Text()
		return nil, mcp.URLElicitationRequiredError{Elicitations: []mcp.ElicitationParams{params}}
	}
	request, err := requestBinding(req)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(sealedState{Key: p.Key, State: state})
	if err != nil {
		return nil, fmt.Errorf("urlelicitation: requestState: %w", err)
	}
	sealed, err := e.sealer.Seal(who, request, payload, e.ttl)
	if err != nil {
		return nil, fmt.Errorf("urlelicitation: seal requestState: %w", err)
	}
	return server.NewInputRequestBuilder(sealed).Elicit(p.Key, params).ToolResult(), nil
}

// Result is the caller's answer to an elicitation, from the specification's
// ElicitResult.
type Result struct {
	// Key is the inputRequests key the elicitation was sent under.
	Key string
	// Action is accept, decline or cancel. Accept means the person agreed to
	// open the URL, not that the flow behind it completed.
	Action mcp.ElicitationResponseAction
	// State is what Elicit was given.
	State []byte
}

// Result reads the answer from a 2026-07-28 retry of the call. It returns nil
// when the call is to be handled as a first call: no requestState (a first
// call, or a retry on an earlier revision), lapsed state (see
// [WithExpired]), or no input response under the key, which the
// specification answers by eliciting again. A returned error is meant to be
// returned by the tool handler as is; it is an invalid params error.
func (e *Elicitor[U]) Result(ctx context.Context, req mcp.CallToolRequest, who U) (*Result, error) {
	if req.Params.RequestState == "" {
		return nil, nil
	}
	request, err := requestBinding(req)
	if err != nil {
		return nil, err
	}
	payload, err := e.sealer.Open(who, request, req.Params.RequestState)
	if err != nil {
		if e.expired(err) {
			return nil, nil
		}
		return nil, invalidParams(fmt.Errorf("%w: %w", ErrInvalidRequestState, err))
	}
	var state sealedState
	if err := json.Unmarshal(payload, &state); err != nil || state.Key == "" {
		return nil, invalidParams(ErrInvalidRequestState)
	}
	response, ok := req.Params.InputResponses[state.Key]
	if !ok {
		return nil, nil
	}
	if response.DecodeFor(mcp.MethodElicitationCreate) != nil || response.Elicitation == nil {
		return nil, invalidParams(fmt.Errorf("%w: %q is not an elicitation result", ErrInvalidResponse, state.Key))
	}
	switch action := response.Elicitation.Action; action {
	case mcp.ElicitationResponseActionAccept, mcp.ElicitationResponseActionDecline, mcp.ElicitationResponseActionCancel:
		return &Result{Key: state.Key, Action: action, State: state.State}, nil
	default:
		return nil, invalidParams(fmt.Errorf("%w: action %q", ErrInvalidResponse, action))
	}
}

// SupportsURL reports whether the caller declared URL mode elicitation: in
// the request from 2026-07-28, at initialize before.
func SupportsURL(ctx context.Context) bool {
	var capabilities *mcp.ClientCapabilities
	if info := server.RequestProtocolInfoFromContext(ctx); info != nil && info.Modern {
		capabilities = info.ClientCapabilities
	} else if session, ok := server.ClientSessionFromContext(ctx).(server.SessionWithClientInfo); ok {
		declared := session.GetClientCapabilities()
		capabilities = &declared
	}
	return capabilities != nil && capabilities.Elicitation != nil && capabilities.Elicitation.URL != nil
}

func fallback(ctx context.Context, p URLParams) *mcp.CallToolResult {
	res := mcp.NewToolResultError(p.Message + " Open " + p.URL + " and then retry.")
	if server.RequestProtocolVersion(ctx) >= mcp.ProtocolVersion20250618 {
		link := mcp.NewResourceLink(p.URL, p.Key, p.Message, "")
		link.Annotations = &mcp.Annotations{Audience: []mcp.Role{mcp.RoleUser}}
		res.Content = append(res.Content, link)
	}
	return res
}

// requestBinding names the originating request a value is sealed for: the
// method, the tool and a digest of its arguments. encoding/json orders map
// keys, so equal arguments bind alike.
func requestBinding(req mcp.CallToolRequest) (string, error) {
	args, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		return "", fmt.Errorf("urlelicitation: arguments: %w", err)
	}
	digest := sha256.Sum256(args)
	return string(mcp.MethodToolsCall) + "\x00" + req.Params.Name + "\x00" + hex.EncodeToString(digest[:]), nil
}

type invalidParamsError struct{ err error }

func (e invalidParamsError) Error() string { return e.err.Error() }
func (e invalidParamsError) Unwrap() error { return e.err }

func (e invalidParamsError) JSONRPCError() mcp.JSONRPCError {
	return mcp.JSONRPCError{
		JSONRPC: mcp.JSONRPC_VERSION,
		Error:   mcp.JSONRPCErrorDetails{Code: mcp.INVALID_PARAMS, Message: e.Error()},
	}
}

// invalidParams marks err for mcp-go to report as -32602 rather than an
// internal error.
func invalidParams(err error) error {
	return server.NewHandlerError(invalidParamsError{err: err})
}

func validParams(p URLParams) error {
	if p.Key == "" || p.Message == "" {
		return errors.New("urlelicitation: an elicitation needs a key and a message")
	}
	return validateURL(p.URL)
}

// validateURL accepts an absolute https URL with a host and no user
// information.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("urlelicitation: %q is not an https URL with a host and no user information", raw)
	}
	return nil
}
