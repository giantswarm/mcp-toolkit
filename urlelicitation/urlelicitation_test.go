package urlelicitation_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/urlelicitation"
)

// fakeSealer binds the state to the user and call in clear text: enough to
// show the Prompter passes them, without depending on a real sealer.
type fakeSealer struct{}

type sealed struct {
	Issuer, Subject, Call string
	Payload               []byte
}

func (fakeSealer) Seal(issuer, subject, call string, payload []byte, _ time.Duration) (string, error) {
	raw, err := json.Marshal(sealed{issuer, subject, call, payload})
	return base64.RawURLEncoding.EncodeToString(raw), err
}

func (fakeSealer) Open(issuer, subject, call, value string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var s sealed
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s.Issuer != issuer || s.Subject != subject || s.Call != call {
		return nil, errors.New("not for this user and call")
	}
	return s.Payload, nil
}

type legacySession struct {
	capabilities mcp.ClientCapabilities
	version      string
}

func (legacySession) Initialize()                                         {}
func (legacySession) Initialized() bool                                   { return true }
func (legacySession) NotificationChannel() chan<- mcp.JSONRPCNotification { return nil }
func (legacySession) SessionID() string                                   { return "s" }
func (legacySession) GetClientInfo() mcp.Implementation                   { return mcp.Implementation{} }
func (legacySession) SetClientInfo(mcp.Implementation)                    {}
func (s legacySession) GetClientCapabilities() mcp.ClientCapabilities     { return s.capabilities }
func (legacySession) SetClientCapabilities(mcp.ClientCapabilities)        {}
func (s legacySession) ProtocolVersion() string                           { return s.version }

const tool = "start_agent"

var (
	argsA = map[string]any{"agent": "a"}
	argsB = map[string]any{"agent": "b"}
)

var (
	alice  = urlelicitation.Identity{Issuer: "https://dex.example", Subject: "alice"}
	bob    = urlelicitation.Identity{Issuer: "https://dex.example", Subject: "bob"}
	urlCap = mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapability{URL: &struct{}{}}}
	signIn = urlelicitation.Request{Key: "sign_in", Message: "Sign in to GitHub for workspace-manager.", URL: "https://github.example/login/oauth/authorize?state=x"}
)

func modern(t *testing.T, capabilities mcp.ClientCapabilities) context.Context {
	return server.WithRequestProtocolInfo(t.Context(), &server.RequestProtocolInfo{Modern: true, ProtocolVersion: "2026-07-28", ClientCapabilities: &capabilities})
}

func legacy(t *testing.T, capabilities mcp.ClientCapabilities) context.Context {
	return server.NewMCPServer("t", "1").WithContext(t.Context(), legacySession{capabilities: capabilities, version: "2025-11-25"})
}

func call(name string, args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

func prompter(t *testing.T) *urlelicitation.Prompter {
	p, err := urlelicitation.New(fakeSealer{}, 0)
	require.NoError(t, err)
	return p
}

func retry(req mcp.CallToolRequest, res *mcp.CallToolResult, key string, action mcp.ElicitationResponseAction) mcp.CallToolRequest {
	req.Params.RequestState = res.RequestState
	req.Params.InputResponses = mcp.InputResponses{key: mcp.NewElicitationInputResponse(mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: action}})}
	return req
}

func TestAskModern(t *testing.T) {
	p := prompter(t)
	ctx := modern(t, urlCap)
	req := call(tool, argsA)

	res, err := p.Ask(ctx, req, alice, signIn, []byte("resume here"))
	require.NoError(t, err)
	require.True(t, res.NeedsInput())
	require.Equal(t, mcp.ElicitationModeURL, res.InputRequests["sign_in"].Elicitation.Mode)
	require.Equal(t, signIn.URL, res.InputRequests["sign_in"].Elicitation.URL)
	require.Empty(t, res.InputRequests["sign_in"].Elicitation.ElicitationID)

	answer, err := p.Answer(ctx, retry(req, res, "sign_in", mcp.ElicitationResponseActionAccept), alice, "sign_in")
	require.NoError(t, err)
	require.Equal(t, mcp.ElicitationResponseActionAccept, answer.Action)
	require.Equal(t, []byte("resume here"), answer.State)

	answer, err = p.Answer(ctx, retry(req, res, "sign_in", mcp.ElicitationResponseActionDecline), alice, "sign_in")
	require.NoError(t, err)
	require.Equal(t, mcp.ElicitationResponseActionDecline, answer.Action)
}

func TestAnswerRefuses(t *testing.T) {
	p := prompter(t)
	ctx := modern(t, urlCap)
	req := call(tool, argsA)
	res, err := p.Ask(ctx, req, alice, signIn, nil)
	require.NoError(t, err)
	accepted := retry(req, res, "sign_in", mcp.ElicitationResponseActionAccept)

	otherCall := retry(call(tool, argsB), res, "sign_in", mcp.ElicitationResponseActionAccept)
	changed := accepted
	changed.Params.RequestState = "x" + accepted.Params.RequestState
	noResponse := accepted
	noResponse.Params.InputResponses = nil

	for name, tc := range map[string]struct {
		req mcp.CallToolRequest
		who urlelicitation.Identity
	}{
		"another user":      {accepted, bob},
		"another call":      {otherCall, alice},
		"changed state":     {changed, alice},
		"no input response": {noResponse, alice},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := p.Answer(ctx, tc.req, tc.who, "sign_in")
			require.ErrorIs(t, err, urlelicitation.ErrInvalidAnswer)
		})
	}
}

func TestAnswerFirstCall(t *testing.T) {
	answer, err := prompter(t).Answer(modern(t, urlCap), call(tool, nil), alice, "sign_in")
	require.NoError(t, err)
	require.Nil(t, answer)
}

func TestAskLegacy(t *testing.T) {
	res, err := prompter(t).Ask(legacy(t, urlCap), call(tool, nil), alice, signIn, []byte("dropped"))
	require.Nil(t, res)
	var required mcp.URLElicitationRequiredError
	require.ErrorAs(t, err, &required)
	require.Len(t, required.Elicitations, 1)
	require.Equal(t, mcp.ElicitationModeURL, required.Elicitations[0].Mode)
	require.Equal(t, signIn.URL, required.Elicitations[0].URL)
	require.NotEmpty(t, required.Elicitations[0].ElicitationID)
}

func TestAskWithoutURLCapability(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"modern": modern(t, mcp.ClientCapabilities{}),
		"legacy": legacy(t, mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapability{}}),
	} {
		t.Run(name, func(t *testing.T) {
			res, err := prompter(t).Ask(ctx, call(tool, nil), alice, signIn, nil)
			require.NoError(t, err)
			require.True(t, res.IsError)
			require.False(t, res.NeedsInput())
			text, ok := mcp.AsTextContent(res.Content[0])
			require.True(t, ok)
			require.Contains(t, text.Text, signIn.URL)
			link, ok := res.Content[1].(mcp.ResourceLink)
			require.True(t, ok)
			require.Equal(t, signIn.URL, link.URI)
		})
	}
}

func TestAskRefusesUnsafeURLs(t *testing.T) {
	for _, raw := range []string{"http://github.example/login", "https://user:pw@github.example/", "/relative", "javascript:alert(1)"} {
		r := signIn
		r.URL = raw
		_, err := prompter(t).Ask(modern(t, urlCap), call("t", nil), alice, r, nil)
		require.Error(t, err, raw)
	}
}

func TestRelayRoundTrip(t *testing.T) {
	p := prompter(t)
	ctx := modern(t, urlCap)
	downstream := server.NewInputRequestBuilder("muster-state").Elicit("muster_sign_in", mcp.ElicitationParams{
		Mode: mcp.ElicitationModeURL, Message: "Sign in to workspace-manager.", URL: "https://muster.example/oauth/start?s=1",
	}).ToolResult()

	d, err := urlelicitation.FromResult(downstream)
	require.NoError(t, err)
	require.Equal(t, "muster_sign_in", d.Key)
	require.Equal(t, "muster-state", d.RequestState)

	req := call(tool, argsA)
	res, err := p.Relay(ctx, req, alice, "authorize_workspace", d)
	require.NoError(t, err)
	require.Equal(t, d.URL, res.InputRequests[urlelicitation.RelayKey].Elicitation.URL)
	require.Equal(t, d.Message, res.InputRequests[urlelicitation.RelayKey].Elicitation.Message)

	resumed, err := p.Resume(ctx, retry(req, res, urlelicitation.RelayKey, mcp.ElicitationResponseActionAccept), alice)
	require.NoError(t, err)
	require.Equal(t, "authorize_workspace", resumed.Target)
	require.Equal(t, mcp.ElicitationResponseActionAccept, resumed.Action)
	require.Equal(t, "muster-state", resumed.Params.RequestState)
	answer := resumed.Params.InputResponses["muster_sign_in"]
	require.NoError(t, answer.DecodeFor(mcp.MethodElicitationCreate))
	require.Equal(t, mcp.ElicitationResponseActionAccept, answer.Elicitation.Action)

	_, err = p.Resume(ctx, retry(req, res, urlelicitation.RelayKey, mcp.ElicitationResponseActionAccept), bob)
	require.ErrorIs(t, err, urlelicitation.ErrInvalidAnswer)
}

func TestFromResult(t *testing.T) {
	d, err := urlelicitation.FromResult(mcp.NewToolResultText("done"))
	require.NoError(t, err)
	require.Nil(t, d)

	form := server.NewInputRequestBuilder("").Elicit("k", mcp.ElicitationParams{Message: "name?"}).ToolResult()
	_, err = urlelicitation.FromResult(form)
	require.Error(t, err)

	plainHTTP := server.NewInputRequestBuilder("").Elicit("k", mcp.ElicitationParams{Mode: mcp.ElicitationModeURL, Message: "m", URL: "http://x.example"}).ToolResult()
	_, err = urlelicitation.FromResult(plainHTTP)
	require.Error(t, err)

	two := server.NewInputRequestBuilder("").
		Elicit("a", mcp.ElicitationParams{Mode: mcp.ElicitationModeURL, Message: "m", URL: "https://a.example"}).
		Elicit("b", mcp.ElicitationParams{Mode: mcp.ElicitationModeURL, Message: "m", URL: "https://b.example"}).ToolResult()
	_, err = urlelicitation.FromResult(two)
	require.Error(t, err)
}
