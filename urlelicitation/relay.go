package urlelicitation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// RelayKey is the inputRequests key a relayed request is asked under.
const RelayKey = "relayed_url"

// Downstream is a URL a server the tool called asks the person to open, and
// what that server needs back on the retry.
type Downstream struct {
	// Key is the server's own inputRequests key.
	Key string
	// Message and URL are the server's own.
	Message string
	URL     string
	// RequestState is the server's own, echoed back to it as is.
	RequestState string
}

// FromResult reads the URL request of a tools/call result a called server
// answered with. It returns nil for a result that needs no input, and an
// error for one that asks for anything but exactly one URL elicitation with
// an https URL.
func FromResult(res *mcp.CallToolResult) (*Downstream, error) {
	if !res.NeedsInput() {
		return nil, nil
	}
	if len(res.InputRequests) != 1 {
		return nil, fmt.Errorf("urlelicitation: the called server asks for %d inputs, want one URL elicitation", len(res.InputRequests))
	}
	for key, input := range res.InputRequests {
		e := input.Elicitation
		if input.Method != mcp.MethodElicitationCreate || e == nil || e.Mode != mcp.ElicitationModeURL {
			return nil, fmt.Errorf("urlelicitation: the called server asks for %s, want a URL elicitation", input.Method)
		}
		if err := ValidURL(e.URL); err != nil {
			return nil, err
		}
		return &Downstream{Key: key, Message: e.Message, URL: e.URL, RequestState: res.RequestState}, nil
	}
	return nil, errors.New("urlelicitation: unreachable")
}

type relayState struct {
	Target       string `json:"t"`
	Key          string `json:"k"`
	RequestState string `json:"s,omitempty"`
}

// Relay asks the tool's caller to open d's URL. target names the called
// server's call, so the retry goes back to it; the state carrying it and d's
// requestState is sealed for the caller and this call.
func (p *Prompter) Relay(ctx context.Context, req mcp.CallToolRequest, who Identity, target string, d *Downstream) (*mcp.CallToolResult, error) {
	state, err := json.Marshal(relayState{Target: target, Key: d.Key, RequestState: d.RequestState})
	if err != nil {
		return nil, fmt.Errorf("urlelicitation: relay state: %w", err)
	}
	return p.Ask(ctx, req, who, Request{Key: RelayKey, Message: d.Message, URL: d.URL}, state)
}

// Resumption is the caller's answer to a relayed request, as the called
// server expects it on the retry.
type Resumption struct {
	// Target is what Relay was given.
	Target string
	// Action is the person's answer.
	Action mcp.ElicitationResponseAction
	// Params go on the retried call to the called server.
	Params mcp.MultiRoundTripParams
}

// Resume reads a 2026-07-28 retry of a call Relay answered. It returns nil
// for a call that carries no requestState. On decline or cancel the tool
// should run nothing and say so; on accept it retries target with Params.
func (p *Prompter) Resume(ctx context.Context, req mcp.CallToolRequest, who Identity) (*Resumption, error) {
	answer, err := p.Answer(ctx, req, who, RelayKey)
	if err != nil || answer == nil {
		return nil, err
	}
	var state relayState
	if err := json.Unmarshal(answer.State, &state); err != nil || state.Target == "" || state.Key == "" {
		return nil, fmt.Errorf("%w: relay state", ErrInvalidAnswer)
	}
	return &Resumption{
		Target: state.Target,
		Action: answer.Action,
		Params: mcp.MultiRoundTripParams{
			InputResponses: mcp.InputResponses{state.Key: mcp.NewElicitationInputResponse(mcp.ElicitationResult{
				ElicitationResponse: mcp.ElicitationResponse{Action: answer.Action},
			})},
			RequestState: state.RequestState,
		},
	}, nil
}
