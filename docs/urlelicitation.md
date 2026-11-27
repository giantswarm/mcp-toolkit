# URL elicitation

The `urlelicitation` package implements MCP URL mode elicitation for mcp-go tools. It supports sensitive out-of-band interactions such as third-party authorization, API key collection and payment flows. Authorization to the MCP server itself belongs to MCP authorization, not elicitation.

For MCP 2026-07-28 clients declaring URL mode support, `Elicit` returns an `InputRequiredResult` with sealed `requestState`. `Result` verifies that state before reading the response. An `accept` action means consent to open the URL; the tool must verify that the external interaction completed.

## Sealing errors

For MCP 2026-07-28 callers declaring URL mode support, every `Sealer.Seal` error fails the call. With mcp-oauth's sealer, a missing user, issuer or subject returns `requeststate.ErrIncompleteIdentity`; a missing OAuth client ID returns `requeststate.ErrMissingClientID`. Either prevents the sealer from satisfying its binding requirements. A missing client ID does not by itself mean that the user is unauthenticated:

```go
elicitor, err := urlelicitation.New(sealer,
    urlelicitation.WithExpired(func(err error) bool {
        return errors.Is(err, requeststate.ErrExpired)
    }),
)
```

The tool must obtain the user from validated authentication context before calling `Elicit` or `Relay`, regardless of protocol revision or capabilities. MCP permits an omitted `requestState`; requiring a sealed continuation on the modern URL mode path is this package's policy. `New` rejects nil sealers, nil options and a nil expired predicate. `Result` never answers a `Sealer.Open` error other than an expired one as a first call; a retry whose state cannot be verified is an invalid-params error.

## Upgrading from v0.10 or v0.11

`WithSealErrorFallback`, available in v0.10.0 and v0.11.0, is removed in the next minor release. Remove it from the options passed to `New`; the constructor above is sufficient. Code using the removed option will not compile until it is removed.

Modern URL mode calls propagate sealing errors even when application state is empty. Callers that relied on a plain URL after `ErrMissingClientID` must supply validated client identity before requesting a sealed continuation. Do not infer that client ID from an access token's resource audience.

## Earlier revisions and capabilities

Clients using MCP 2025-11-25 with URL mode support receive `URLElicitationRequiredError`. Clients without URL mode support receive the tool error containing the URL, with a `resource_link` from MCP 2025-06-18. These paths do not call the sealer or preserve application state. Tools serving them must authenticate the caller themselves and be able to restart from their arguments and server-side state.

## URL safety

URLs must use HTTPS without user information. This validation does not establish that a URL is safe: the application must exclude credentials, personal information and pre-authenticated access URLs. The web flow must verify that the person completing the interaction is the person who initiated it. Never treat an elicitation response as proof of authentication or completion.

See the [MCP elicitation specification](https://modelcontextprotocol.io/specification/2026-07-28/client/elicitation) and [multi round-trip requests](https://modelcontextprotocol.io/specification/2026-07-28/basic/patterns/mrtr).
