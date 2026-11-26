package urlelicitation_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-toolkit/urlelicitation"
)

var (
	errLapsed          = errors.New("lapsed")
	errMissingClientID = errors.New("missing client ID")
)

// user stands in for the validated user type, such as mcp-oauth's
// *providers.UserInfo.
type user struct{ Issuer, Subject string }

// fakeSealer binds the state to the user and request in clear text: enough to
// show the Elicitor passes them, without depending on a real sealer.
type fakeSealer struct {
	ttl       time.Duration
	lapsed    bool
	sealError error
	openError error
	sealIDs   int
}

type sealed struct {
	User    user
	Request string
	Payload []byte
}

func (f *fakeSealer) Seal(u user, request string, payload []byte, ttl time.Duration) (string, error) {
	if f.sealError != nil {
		return "", f.sealError
	}
	f.ttl = ttl
	f.sealIDs++
	raw, err := json.Marshal(sealed{u, request, payload})
	return base64.RawURLEncoding.EncodeToString(raw), err
}

func (f *fakeSealer) Open(u user, request, value string) ([]byte, error) {
	if f.lapsed {
		return nil, errLapsed
	}
	if f.openError != nil {
		return nil, f.openError
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var s sealed
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s.User != u || s.Request != request {
		return nil, errors.New("not for this user and request")
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
	alice  = user{Issuer: "https://dex.example", Subject: "alice"}
	bob    = user{Issuer: "https://dex.example", Subject: "bob"}
	urlCap = mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapability{URL: &struct{}{}}}
	signIn = urlelicitation.URLParams{Key: "sign_in", Message: "Sign in to GitHub for workspace-manager.", URL: "https://github.example/login/oauth/authorize?state=x"}
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

func elicitor(t *testing.T, sealer urlelicitation.Sealer[user], opts ...urlelicitation.Option) *urlelicitation.Elicitor[user] {
	e, err := urlelicitation.New(sealer, opts...)
	require.NoError(t, err)
	return e
}

func retry(req mcp.CallToolRequest, res *mcp.CallToolResult, key string, action mcp.ElicitationResponseAction) mcp.CallToolRequest {
	req.Params.RequestState = res.RequestState
	req.Params.InputResponses = mcp.InputResponses{key: mcp.NewElicitationInputResponse(mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: action}})}
	return req
}

func requireInvalidParams(t *testing.T, err error, sentinel error) {
	t.Helper()
	require.ErrorIs(t, err, sentinel)
	var provider server.JSONRPCErrorProvider
	require.ErrorAs(t, err, &provider)
	require.Equal(t, mcp.INVALID_PARAMS, provider.JSONRPCError().Error.Code)
}

func TestElicitModern(t *testing.T) {
	e := elicitor(t, &fakeSealer{})
	ctx := modern(t, urlCap)
	req := call(tool, argsA)

	res, err := e.Elicit(ctx, req, alice, signIn, []byte("resume here"))
	require.NoError(t, err)
	require.True(t, res.NeedsInput())
	require.Equal(t, mcp.ElicitationModeURL, res.InputRequests["sign_in"].Elicitation.Mode)
	require.Equal(t, signIn.URL, res.InputRequests["sign_in"].Elicitation.URL)
	require.Empty(t, res.InputRequests["sign_in"].Elicitation.ElicitationID)

	result, err := e.Result(ctx, retry(req, res, "sign_in", mcp.ElicitationResponseActionAccept), alice)
	require.NoError(t, err)
	require.Equal(t, "sign_in", result.Key)
	require.Equal(t, mcp.ElicitationResponseActionAccept, result.Action)
	require.Equal(t, []byte("resume here"), result.State)

	result, err = e.Result(ctx, retry(req, res, "sign_in", mcp.ElicitationResponseActionDecline), alice)
	require.NoError(t, err)
	require.Equal(t, mcp.ElicitationResponseActionDecline, result.Action)
}

func TestElicitTTL(t *testing.T) {
	for name, tc := range map[string]struct {
		opts []urlelicitation.Option
		want time.Duration
	}{
		"default":     {nil, urlelicitation.DefaultTTL},
		"set":         {[]urlelicitation.Option{urlelicitation.WithTTL(time.Minute)}, time.Minute},
		"nonpositive": {[]urlelicitation.Option{urlelicitation.WithTTL(-time.Minute)}, urlelicitation.DefaultTTL},
	} {
		t.Run(name, func(t *testing.T) {
			sealer := &fakeSealer{}
			_, err := elicitor(t, sealer, tc.opts...).Elicit(modern(t, urlCap), call(tool, nil), alice, signIn, nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, sealer.ttl)
		})
	}
}

func TestElicitRefusesRelayKey(t *testing.T) {
	p := signIn
	p.Key = urlelicitation.RelayKey
	_, err := elicitor(t, &fakeSealer{}).Elicit(modern(t, urlCap), call(tool, nil), alice, p, nil)
	require.Error(t, err)
}

func TestResultRejects(t *testing.T) {
	e := elicitor(t, &fakeSealer{})
	ctx := modern(t, urlCap)
	req := call(tool, argsA)
	res, err := e.Elicit(ctx, req, alice, signIn, nil)
	require.NoError(t, err)
	accepted := retry(req, res, "sign_in", mcp.ElicitationResponseActionAccept)

	otherCall := retry(call(tool, argsB), res, "sign_in", mcp.ElicitationResponseActionAccept)
	changed := accepted
	changed.Params.RequestState = "x" + accepted.Params.RequestState
	unknownAction := retry(req, res, "sign_in", "maybe")

	for name, tc := range map[string]struct {
		req      mcp.CallToolRequest
		who      user
		sentinel error
	}{
		"another user":   {accepted, bob, urlelicitation.ErrInvalidRequestState},
		"another call":   {otherCall, alice, urlelicitation.ErrInvalidRequestState},
		"changed state":  {changed, alice, urlelicitation.ErrInvalidRequestState},
		"unknown action": {unknownAction, alice, urlelicitation.ErrInvalidResponse},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := e.Result(ctx, tc.req, tc.who)
			requireInvalidParams(t, err, tc.sentinel)
		})
	}
}

func TestResultAsFirstCall(t *testing.T) {
	sealer := &fakeSealer{}
	e := elicitor(t, sealer, urlelicitation.WithExpired(func(err error) bool { return errors.Is(err, errLapsed) }))
	ctx := modern(t, urlCap)
	req := call(tool, argsA)
	res, err := e.Elicit(ctx, req, alice, signIn, nil)
	require.NoError(t, err)

	noResponse := retry(req, res, "sign_in", mcp.ElicitationResponseActionAccept)
	noResponse.Params.InputResponses = nil

	for name, req := range map[string]mcp.CallToolRequest{
		"no requestState": call(tool, argsA),
		"no response":     noResponse,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := e.Result(ctx, req, alice)
			require.NoError(t, err)
			require.Nil(t, result)
		})
	}

	t.Run("lapsed", func(t *testing.T) {
		sealer.lapsed = true
		result, err := e.Result(ctx, retry(req, res, "sign_in", mcp.ElicitationResponseActionAccept), alice)
		require.NoError(t, err)
		require.Nil(t, result)
	})
}

func TestResultLapsedWithoutExpiredOption(t *testing.T) {
	sealer := &fakeSealer{}
	e := elicitor(t, sealer)
	ctx := modern(t, urlCap)
	req := call(tool, argsA)
	res, err := e.Elicit(ctx, req, alice, signIn, nil)
	require.NoError(t, err)

	sealer.lapsed = true
	_, err = e.Result(ctx, retry(req, res, "sign_in", mcp.ElicitationResponseActionAccept), alice)
	requireInvalidParams(t, err, urlelicitation.ErrInvalidRequestState)
}

func TestElicitLegacy(t *testing.T) {
	sealer := &fakeSealer{}
	e := elicitor(t, sealer)
	ids := map[string]bool{}
	for range 2 {
		res, err := e.Elicit(legacy(t, urlCap), call(tool, nil), alice, signIn, []byte("dropped"))
		require.Nil(t, res)
		var required mcp.URLElicitationRequiredError
		require.ErrorAs(t, err, &required)
		require.Len(t, required.Elicitations, 1)
		require.Equal(t, mcp.ElicitationModeURL, required.Elicitations[0].Mode)
		require.Equal(t, signIn.URL, required.Elicitations[0].URL)
		require.NotEmpty(t, required.Elicitations[0].ElicitationID)
		ids[required.Elicitations[0].ElicitationID] = true
	}
	require.Len(t, ids, 2)
	require.Zero(t, sealer.sealIDs)
}

func TestElicitWithoutURLCapability(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"modern": modern(t, mcp.ClientCapabilities{}),
		"legacy": legacy(t, mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapability{}}),
	} {
		t.Run(name, func(t *testing.T) {
			res, err := elicitor(t, &fakeSealer{}).Elicit(ctx, call(tool, nil), alice, signIn, nil)
			require.NoError(t, err)
			require.True(t, res.IsError)
			require.False(t, res.NeedsInput())
			text, ok := mcp.AsTextContent(res.Content[0])
			require.True(t, ok)
			require.Contains(t, text.Text, signIn.URL)
			link, ok := res.Content[1].(mcp.ResourceLink)
			require.True(t, ok)
			require.Equal(t, signIn.URL, link.URI)
			require.Empty(t, link.MIMEType)
		})
	}
}

func TestElicitRefusesUnsafeURLs(t *testing.T) {
	for _, raw := range []string{"http://github.example/login", "https://user:pw@github.example/", "/relative", "javascript:alert(1)"} {
		p := signIn
		p.URL = raw
		_, err := elicitor(t, &fakeSealer{}).Elicit(modern(t, urlCap), call("t", nil), alice, p, nil)
		require.Error(t, err, raw)
	}
}

func TestRelayRoundTrip(t *testing.T) {
	e := elicitor(t, &fakeSealer{})
	ctx := modern(t, urlCap)
	called := server.NewInputRequestBuilder("muster-state").Elicit("muster_sign_in", mcp.ElicitationParams{
		Mode: mcp.ElicitationModeURL, Message: "Sign in to GitHub for workspace-manager.", URL: "https://muster.example/oauth/start?s=1",
	}).ToolResult()

	relayed, err := urlelicitation.FromInputRequired(called)
	require.NoError(t, err)
	require.Equal(t, "muster_sign_in", relayed.Key)
	require.Equal(t, "muster-state", relayed.RequestState)

	req := call(tool, argsA)
	res, err := e.Relay(ctx, req, alice, "authorize_workspace", relayed)
	require.NoError(t, err)
	require.Equal(t, relayed.URL, res.InputRequests[urlelicitation.RelayKey].Elicitation.URL)
	require.Equal(t, relayed.Message, res.InputRequests[urlelicitation.RelayKey].Elicitation.Message)

	result, err := e.Result(ctx, retry(req, res, urlelicitation.RelayKey, mcp.ElicitationResponseActionAccept), alice)
	require.NoError(t, err)
	require.Equal(t, urlelicitation.RelayKey, result.Key)
	r, err := result.Retry()
	require.NoError(t, err)
	require.Equal(t, "authorize_workspace", r.Target)
	require.Equal(t, mcp.ElicitationResponseActionAccept, r.Action)
	require.Equal(t, "muster-state", r.Params.RequestState)
	answer := r.Params.InputResponses["muster_sign_in"]
	require.NoError(t, answer.DecodeFor(mcp.MethodElicitationCreate))
	require.Equal(t, mcp.ElicitationResponseActionAccept, answer.Elicitation.Action)

	_, err = e.Result(ctx, retry(req, res, urlelicitation.RelayKey, mcp.ElicitationResponseActionAccept), bob)
	requireInvalidParams(t, err, urlelicitation.ErrInvalidRequestState)
}

func TestRetryOfOwnElicitation(t *testing.T) {
	e := elicitor(t, &fakeSealer{})
	ctx := modern(t, urlCap)
	req := call(tool, argsA)
	res, err := e.Elicit(ctx, req, alice, signIn, nil)
	require.NoError(t, err)

	result, err := e.Result(ctx, retry(req, res, "sign_in", mcp.ElicitationResponseActionAccept), alice)
	require.NoError(t, err)
	require.Equal(t, "sign_in", result.Key)
	_, err = result.Retry()
	require.Error(t, err)
}

func TestFromInputRequired(t *testing.T) {
	relayed, err := urlelicitation.FromInputRequired(mcp.NewToolResultText("done"))
	require.NoError(t, err)
	require.Nil(t, relayed)

	form := server.NewInputRequestBuilder("").Elicit("k", mcp.ElicitationParams{Message: "name?"}).ToolResult()
	_, err = urlelicitation.FromInputRequired(form)
	require.Error(t, err)

	plainHTTP := server.NewInputRequestBuilder("").Elicit("k", mcp.ElicitationParams{Mode: mcp.ElicitationModeURL, Message: "m", URL: "http://x.example"}).ToolResult()
	_, err = urlelicitation.FromInputRequired(plainHTTP)
	require.Error(t, err)

	two := server.NewInputRequestBuilder("").
		Elicit("a", mcp.ElicitationParams{Mode: mcp.ElicitationModeURL, Message: "m", URL: "https://a.example"}).
		Elicit("b", mcp.ElicitationParams{Mode: mcp.ElicitationModeURL, Message: "m", URL: "https://b.example"}).ToolResult()
	_, err = urlelicitation.FromInputRequired(two)
	require.Error(t, err)
}

func TestNewInfersUserType(t *testing.T) {
	e, err := urlelicitation.New(&fakeSealer{})
	require.NoError(t, err)
	require.IsType(t, &urlelicitation.Elicitor[user]{}, e)
}

func TestNewRefusesNilSealer(t *testing.T) {
	_, err := urlelicitation.New[user](nil)
	require.Error(t, err)
	_, err = urlelicitation.New((*fakeSealer)(nil))
	require.Error(t, err)
}

func TestNewRefusesNilOptions(t *testing.T) {
	for name, option := range map[string]urlelicitation.Option{
		"nil option":                  nil,
		"nil expired predicate":       urlelicitation.WithExpired(nil),
		"nil sealing error predicate": urlelicitation.WithSealErrorFallback(nil),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := urlelicitation.New(&fakeSealer{}, option)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}

func TestElicitSealErrorFallback(t *testing.T) {
	options := []urlelicitation.Option{urlelicitation.WithSealErrorFallback(func(err error) bool {
		return errors.Is(err, errMissingClientID)
	})}
	for name, test := range map[string]struct {
		sealError    error
		options      []urlelicitation.Option
		state        []byte
		wantFallback bool
	}{
		"disabled":               {errMissingClientID, nil, nil, false},
		"missing client":         {errMissingClientID, options, nil, true},
		"empty state":            {errMissingClientID, options, []byte{}, true},
		"wrapped missing client": {fmt.Errorf("adapter: %w", errMissingClientID), options, nil, true},
		"key failure":            {errors.New("active key unavailable"), options, nil, false},
		"missing principal":      {errors.New("authenticated user is required"), options, nil, false},
		"continuation required":  {errMissingClientID, options, []byte("resume here"), false},
	} {
		t.Run(name, func(t *testing.T) {
			sealer := &fakeSealer{sealError: test.sealError}
			result, err := elicitor(t, sealer, test.options...).Elicit(modern(t, urlCap), call(tool, nil), alice, signIn, test.state)
			if !test.wantFallback {
				require.ErrorIs(t, err, test.sealError)
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.False(t, result.NeedsInput())
			require.Empty(t, result.RequestState)
			require.Empty(t, result.InputRequests)
			require.Len(t, result.Content, 2)
			message, ok := mcp.AsTextContent(result.Content[0])
			require.True(t, ok)
			require.Contains(t, message.Text, signIn.URL)
			link, ok := result.Content[1].(mcp.ResourceLink)
			require.True(t, ok)
			require.Equal(t, signIn.URL, link.URI)
		})
	}
}

func TestRelayPreservesSealingError(t *testing.T) {
	sealer := &fakeSealer{sealError: errMissingClientID}
	elicitor := elicitor(t, sealer, urlelicitation.WithSealErrorFallback(func(err error) bool {
		return errors.Is(err, errMissingClientID)
	}))
	result, err := elicitor.Relay(modern(t, urlCap), call(tool, argsA), alice, "downstream-tool", &urlelicitation.Relayed{
		Key: "downstream-key", Message: signIn.Message, URL: signIn.URL, RequestState: "downstream-state",
	})
	require.ErrorIs(t, err, errMissingClientID)
	require.Nil(t, result)
}

func TestResultDoesNotUseSealErrorFallback(t *testing.T) {
	sealer := &fakeSealer{}
	elicitor := elicitor(t, sealer, urlelicitation.WithSealErrorFallback(func(err error) bool {
		t.Fatal("a state verification error must not select a URL fallback")
		return true
	}))
	ctx := modern(t, urlCap)
	request := call(tool, argsA)
	result, err := elicitor.Elicit(ctx, request, alice, signIn, nil)
	require.NoError(t, err)
	sealer.openError = errMissingClientID
	_, err = elicitor.Result(ctx, retry(request, result, "sign_in", mcp.ElicitationResponseActionAccept), alice)
	requireInvalidParams(t, err, urlelicitation.ErrInvalidRequestState)
	require.ErrorIs(t, err, errMissingClientID)
}
