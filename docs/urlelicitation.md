# URL elicitation

The `urlelicitation` package implements MCP URL mode elicitation for mcp-go tools. It supports sensitive out-of-band interactions such as third-party authorization, API key collection and payment flows. Authorization to the MCP server itself belongs to MCP authorization, not elicitation.

For MCP 2026-07-28 clients declaring URL mode support, `Elicit` returns an `InputRequiredResult` with sealed `requestState`. `Result` verifies that state before reading the response. An `accept` action means consent to open the URL; the tool must verify that the external interaction completed.

## Sealing error fallback

`WithSealErrorFallback` is an optional local policy. Its predicate selects `Sealer.Seal` errors for which a call with empty application state may return a tool error containing the URL and a `resource_link`. The client receives no `inputRequests` or `requestState`; the person opens the URL and retries the tool as a new call. This is not an MCP or OAuth error classification.

For mcp-oauth's sealer, configure only the missing client ID error:

```go
elicitor, err := urlelicitation.New(sealer,
    urlelicitation.WithExpired(func(err error) bool {
        return errors.Is(err, requeststate.ErrExpired)
    }),
    urlelicitation.WithSealErrorFallback(func(err error) bool {
        return errors.Is(err, requeststate.ErrMissingClientID)
    }),
)
```

The tool must obtain the user from validated authentication context. A missing user, issuer or subject returns `requeststate.ErrIncompleteIdentity` and must remain an error. Key failures and other operational errors must also remain errors. `New` rejects nil sealers, nil options and nil error predicates.

The sealing error fallback is refused when application state is nonempty. `Relay` always carries a continuation containing the downstream target, input key and request state, so it propagates sealing errors. `Result` never applies this fallback to `Sealer.Open` errors; a retry whose state cannot be verified remains an invalid-params error.

## Earlier revisions and capabilities

Clients using MCP 2025-11-25 with URL mode support receive `URLElicitationRequiredError`. Clients without URL mode support receive the tool error containing the URL, with a `resource_link` from MCP 2025-06-18. These paths do not preserve application state. Tools serving them must be able to restart from their arguments and server-side state.

## URL safety

URLs must use HTTPS without user information. This validation does not establish that a URL is safe: the application must exclude credentials, personal information and pre-authenticated access URLs. The web flow must verify that the person completing the interaction is the person who initiated it. Never treat an elicitation response as proof of authentication or completion.

See the [MCP elicitation specification](https://modelcontextprotocol.io/specification/2026-07-28/client/elicitation) and [multi round-trip requests](https://modelcontextprotocol.io/specification/2026-07-28/basic/patterns/mrtr).
