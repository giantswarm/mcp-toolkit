package urlelicitation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// RelayKey is the inputRequests key a relayed elicitation is sent under.
const RelayKey = "relayed_url"

// Relayed is a URL elicitation a server the tool called sent, and what that
// server needs back on the retry.
type Relayed struct {
	// Key is the called server's own inputRequests key.
	Key string
	// Message and URL are the called server's own.
	Message string
	URL     string
	// RequestState is the called server's own, echoed back to it as is.
	RequestState string
}

// FromInputRequired reads the URL elicitation of a tools/call result a
// called server answered with. It returns nil for a result that needs no
// input, and an error for one that asks for anything but exactly one URL
// elicitation with an https URL.
func FromInputRequired(res *mcp.CallToolResult) (*Relayed, error) {
	if !res.NeedsInput() {
		return nil, nil
	}
	if len(res.InputRequests) != 1 {
		return nil, fmt.Errorf("urlelicitation: the called server asks for %d inputs, want one URL elicitation", len(res.InputRequests))
	}
	var key string
	var input mcp.InputRequest
	for k, in := range res.InputRequests {
		key, input = k, in
	}
	e := input.Elicitation
	if input.Method != mcp.MethodElicitationCreate || e == nil || e.Mode != mcp.ElicitationModeURL {
		return nil, fmt.Errorf("urlelicitation: the called server asks for %s, want a URL elicitation", input.Method)
	}
	if err := validateURL(e.URL); err != nil {
		return nil, err
	}
	return &Relayed{Key: key, Message: e.Message, URL: e.URL, RequestState: res.RequestState}, nil
}

type relayState struct {
	Target       string `json:"t"`
	Key          string `json:"k"`
	RequestState string `json:"s,omitempty"`
}

// Relay asks the tool's caller to open r's URL. target names the called
// server's call, so the retry goes back to it; the state carrying it and r's
// requestState is sealed for the caller and this request. Relay only third-party
// sign-ins: a called server's sign-in to itself is MCP authorization, not URL
// elicitation.
func (e *Elicitor[U]) Relay(ctx context.Context, req mcp.CallToolRequest, who U, target string, r *Relayed) (*mcp.CallToolResult, error) {
	state, err := json.Marshal(relayState{Target: target, Key: r.Key, RequestState: r.RequestState})
	if err != nil {
		return nil, fmt.Errorf("urlelicitation: relay state: %w", err)
	}
	return e.elicit(ctx, req, who, URLParams{Key: RelayKey, Message: r.Message, URL: r.URL}, state)
}

// Retry is the caller's answer to a relayed elicitation, as the called server
// expects it on the retry.
type Retry struct {
	// Target is what Relay was given.
	Target string
	// Action is the person's answer.
	Action mcp.ElicitationResponseAction
	// Params go on the retried call to the called server.
	Params mcp.MultiRoundTripParams
}

// Retry turns the result of an elicitation Relay sent, under [RelayKey],
// into the retry of the called server's call. On decline or cancel the tool
// runs nothing and says so; on accept it retries Target with Params, and the
// called server decides whether its flow completed.
func (r *Result) Retry() (*Retry, error) {
	if r.Key != RelayKey {
		return nil, fmt.Errorf("urlelicitation: the result under %q is not a relayed elicitation", r.Key)
	}
	var state relayState
	if err := json.Unmarshal(r.State, &state); err != nil || state.Target == "" || state.Key == "" {
		return nil, invalidParams(fmt.Errorf("%w: relay state", ErrInvalidRequestState))
	}
	return &Retry{
		Target: state.Target,
		Action: r.Action,
		Params: mcp.MultiRoundTripParams{
			InputResponses: mcp.InputResponses{state.Key: mcp.NewElicitationInputResponse(mcp.ElicitationResult{
				ElicitationResponse: mcp.ElicitationResponse{Action: r.Action},
			})},
			RequestState: state.RequestState,
		},
	}, nil
}
