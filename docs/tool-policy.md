# Tool policy logging and migration

`toolpolicy.Options.User` reads an already-validated identity's log representation from the request context. It returns a `slog.LogValuer`; toolkit records that value under `user` alongside `tool` and `outcome`. Authentication, authorization, log fields and redaction belong to the application. Toolkit does not validate this callback's identity or remove its fields. A nil callback or result omits the user. An empty slog group is omitted by the standard handlers; a zero `slog.Value` is logged as `null`.

mcp-oauth's `*providers.UserInfo` implements this interface. It selects issuer, subject and supplied current actor fields, excluding profile fields such as email, name and groups. Identifier values may themselves contain personal data, so protect the logs and limit retention. Its formatter does not authorize a request. The `user.*` keys are an application audit schema, not an MCP or A2A wire extension.

## Migrating from `toolpolicy.Identity`

The exported `toolpolicy.Identity` type and the `func(context.Context) (toolpolicy.Identity, bool)` callback are removed. Replace a copying adapter with a callback returning the validated identity as a `slog.LogValuer`:

```go
func auditUser(ctx context.Context) slog.LogValuer {
	userInfo, ok := handler.UserInfoFromContext(ctx)
	if !ok || userInfo == nil {
		return nil
	}
	return userInfo
}

options := toolpolicy.Options{Logger: logger, User: auditUser}
```

This example imports `context`, `log/slog`, `github.com/giantswarm/mcp-oauth/handler` and `github.com/giantswarm/mcp-toolkit/toolpolicy`. Upgrade mcp-oauth to a release containing [giantswarm/mcp-oauth#777](https://github.com/giantswarm/mcp-oauth/pull/777) before using `UserInfo` directly. Other authentication libraries can supply their own `slog.LogValuer` without depending on mcp-oauth. Update applications such as model-manager's `validatedToolIdentity` adapter when upgrading toolkit; pinned older dependencies continue to use their existing API.

Update JSON log queries and dashboards at the same time:

| Previous field | Field with `providers.UserInfo` |
| --- | --- |
| `issuer` | `user.issuer` |
| `subject` | `user.subject` |
| `actor_issuer` | `user.actor.issuer` |
| `actor_subject` | `user.actor.subject` |

These are nested JSON objects, not literal dotted keys. Missing actor fields are omitted independently, and do not suppress the top-level user. `tool`, `outcome`, and the `tool_call` message are unchanged. A custom callback may choose another value shape; its application owns the corresponding queries.

The [Kubernetes example](kubernetes.md) separates the MCP credential from the downstream credential and fails closed for unsupported authentication sources. A user log value is never a credential source.
