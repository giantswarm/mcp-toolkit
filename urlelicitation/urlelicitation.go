// Package urlelicitation asks a tool's caller to open a URL, such as a
// third-party sign-in, with MCP URL mode elicitation, and passes on such a
// request from a server the tool called.
//
// [Prompter.Ask] answers each caller the way its protocol revision defines:
//
//   - 2026-07-28 with elicitation.url declared: an InputRequiredResult with
//     the elicitation and a sealed requestState.
//   - 2025-11-25 with elicitation.url declared: URLElicitationRequiredError
//     (-32042) with a sealed elicitationId. No completion notification is
//     sent (the specification makes it optional), so the caller retries when
//     the person says they are done.
//   - Otherwise: a tool error with the message and the URL, and the URL as a
//     resource_link from 2025-06-18.
//
// State is sealed by a [Sealer], bound to the validated caller and to the
// call (tool name and arguments), so a caller cannot change it or replay it
// for another person or call. mcp-oauth's requeststate.Sealer implements it.
package urlelicitation

import (
	"context"
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

// Sealer seals state for a user and a call and opens it again.
type Sealer interface {
	Seal(issuer, subject, call string, payload []byte, ttl time.Duration) (string, error)
	Open(issuer, subject, call, sealed string) ([]byte, error)
}

// Identity is the validated caller the state is bound to.
type Identity struct {
	Issuer  string
	Subject string
}

// Request is one URL the caller is asked to open.
type Request struct {
	// Key names the request in inputRequests; the answer comes back under it.
	Key string
	// Message tells the person why, naming the server that asks.
	Message string
	// URL is the https URL to open. It must not be pre-authenticated.
	URL string
}

// ErrInvalidAnswer is a retry whose requestState or input response is not
// one this server issued for the caller and call.
var ErrInvalidAnswer = errors.New("urlelicitation: invalid answer")

// DefaultTTL is how long a caller has to answer.
const DefaultTTL = 15 * time.Minute

// Prompter asks callers to open URLs.
type Prompter struct {
	sealer Sealer
	ttl    time.Duration
}

// New returns a Prompter that seals with sealer. A nonpositive ttl takes
// [DefaultTTL].
func New(sealer Sealer, ttl time.Duration) (*Prompter, error) {
	if sealer == nil {
		return nil, errors.New("urlelicitation: a sealer is required")
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Prompter{sealer: sealer, ttl: ttl}, nil
}

// Ask answers the call with r, carrying state back on a 2026-07-28 retry. See
// the package documentation for the answer per revision. A returned error is
// meant to be returned by the tool handler as is.
func (p *Prompter) Ask(ctx context.Context, req mcp.CallToolRequest, who Identity, r Request, state []byte) (*mcp.CallToolResult, error) {
	if err := validRequest(r); err != nil {
		return nil, err
	}
	if !SupportsURL(ctx) {
		return fallback(ctx, r), nil
	}
	call, err := callBinding(req)
	if err != nil {
		return nil, err
	}
	params := mcp.ElicitationParams{Mode: mcp.ElicitationModeURL, Message: r.Message, URL: r.URL}
	if server.IsModernRequest(ctx) {
		sealed, err := p.sealer.Seal(who.Issuer, who.Subject, call, state, p.ttl)
		if err != nil {
			return nil, fmt.Errorf("urlelicitation: seal requestState: %w", err)
		}
		return server.NewInputRequestBuilder(sealed).Elicit(r.Key, params).ToolResult(), nil
	}
	id, err := p.sealer.Seal(who.Issuer, who.Subject, call+"\x00"+r.URL, nil, p.ttl)
	if err != nil {
		return nil, fmt.Errorf("urlelicitation: seal elicitationId: %w", err)
	}
	params.ElicitationID = id
	return nil, mcp.URLElicitationRequiredError{Elicitations: []mcp.ElicitationParams{params}}
}

// Answer is the caller's answer to a request [Prompter.Ask] made.
type Answer struct {
	// Action is accept, decline or cancel.
	Action mcp.ElicitationResponseAction
	// State is what Ask was given.
	State []byte
}

// Answer reads the answer under key from a 2026-07-28 retry of the call. It
// returns nil for a call that carries no requestState: a first call, or a
// retry on an earlier revision, which runs as a new call.
func (p *Prompter) Answer(ctx context.Context, req mcp.CallToolRequest, who Identity, key string) (*Answer, error) {
	if req.Params.RequestState == "" {
		return nil, nil
	}
	call, err := callBinding(req)
	if err != nil {
		return nil, err
	}
	state, err := p.sealer.Open(who.Issuer, who.Subject, call, req.Params.RequestState)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAnswer, err)
	}
	response, ok := req.Params.InputResponses[key]
	if !ok || response.DecodeFor(mcp.MethodElicitationCreate) != nil || response.Elicitation == nil {
		return nil, fmt.Errorf("%w: no elicitation answer under %q", ErrInvalidAnswer, key)
	}
	switch action := response.Elicitation.Action; action {
	case mcp.ElicitationResponseActionAccept, mcp.ElicitationResponseActionDecline, mcp.ElicitationResponseActionCancel:
		return &Answer{Action: action, State: state}, nil
	default:
		return nil, fmt.Errorf("%w: action %q", ErrInvalidAnswer, action)
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

func fallback(ctx context.Context, r Request) *mcp.CallToolResult {
	res := mcp.NewToolResultError(r.Message + " Open " + r.URL + " and then retry.")
	if server.RequestProtocolVersion(ctx) >= mcp.ProtocolVersion20250618 {
		link := mcp.NewResourceLink(r.URL, r.Key, r.Message, "text/html")
		link.Annotations = &mcp.Annotations{Audience: []mcp.Role{mcp.RoleUser}}
		res.Content = append(res.Content, link)
	}
	return res
}

// callBinding names the call a value is sealed for: the tool and a digest of
// its arguments. encoding/json orders map keys, so equal arguments bind alike.
func callBinding(req mcp.CallToolRequest) (string, error) {
	args, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		return "", fmt.Errorf("urlelicitation: arguments: %w", err)
	}
	digest := sha256.Sum256(args)
	return req.Params.Name + "\x00" + hex.EncodeToString(digest[:]), nil
}

func validRequest(r Request) error {
	if r.Key == "" || r.Message == "" {
		return errors.New("urlelicitation: a request needs a key and a message")
	}
	return ValidURL(r.URL)
}

// ValidURL accepts an absolute https URL with a host and no user information.
func ValidURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("urlelicitation: %q is not an https URL with a host and no user information", raw)
	}
	return nil
}
