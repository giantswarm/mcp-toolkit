# Kubernetes caller clients

The core packages and the optional [`k8s`](../k8s) module work together like this. The core
packages apply tool policy. The `k8s` module makes every Kubernetes call with a caller-specific
credential, so the caller's RBAC decides what the tool may read.

`k8s` is a separate Go module, so client-go stays out of servers that do not use it:

```sh
go get github.com/giantswarm/mcp-toolkit github.com/giantswarm/mcp-toolkit/k8s
```

The example uses mcp-oauth's locally issued opaque MCP access tokens (`TokenSourceOAuth`),
with `RequireResourceAccessTokens: true` in the OAuth server configuration. Run the MCP
transport inside `handler.ValidateToken` so the request context is authenticated. The
upstream login must separately obtain an OpenID Connect ID token whose issuer and audience
the Kubernetes API server is configured to accept.

`handler.UserInfoFromContext` supplies the validated identity. `handler.ProviderIDTokenFromContext`
resolves the separate stored provider ID token for the supported authentication source.
The helper below rejects every other source before resolving credentials, including forwarded
ID tokens and trusted-issuer bearers: for those sources the accessor can return the incoming
bearer itself. [MCP forbids passing that bearer to an upstream API](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations#access-token-privilege-restriction).
Servers supporting other sources need a separate downstream token acquisition or exchange path.

`*providers.UserInfo` implements `slog.LogValuer` and logs issuer, subject and supplied current
actor fields, excluding profile fields. Subject values may themselves be email addresses.
mcp-toolkit does not import mcp-oauth; see the [logging contract and migration](tool-policy.md).

```go
import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/giantswarm/mcp-oauth/handler"
	"github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-toolkit/k8s"
	policy "github.com/giantswarm/mcp-toolkit/middleware/mcpgo"
	"github.com/giantswarm/mcp-toolkit/toolpolicy"
)

// user is the user mcp-oauth validated, or nil.
func user(ctx context.Context) slog.LogValuer {
	userInfo, ok := handler.UserInfoFromContext(ctx)
	if !ok || userInfo == nil {
		return nil
	}
	return userInfo
}

// userClients requires a stored provider credential separate from the MCP bearer.
func userClients(ctx context.Context, users *k8s.UserClients) (*k8s.Clients, error) {
	userInfo, ok := handler.UserInfoFromContext(ctx)
	if !ok || userInfo == nil || userInfo.TokenSource != providers.TokenSourceOAuth {
		return nil, handler.ErrProviderIDTokenUnavailable
	}
	idToken, err := handler.ProviderIDTokenFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return users.ForToken(idToken)
}

func newServer(base *rest.Config, application kubernetes.Interface, logger *slog.Logger) (*mcpserver.MCPServer, error) {
	users, err := k8s.NewUserClients(base, nil)
	if err != nil {
		return nil, err
	}
	s := mcpserver.NewMCPServer("my-mcp", "1.0.0",
		policy.New(toolpolicy.Options{MaxResultBytes: 128 << 10, Logger: logger, User: user}),
	)

	// The user's own RBAC governs the read.
	s.AddTool(mcp.NewTool("list_pods", mcp.WithString("namespace", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			namespace, err := request.RequireString("namespace")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			clients, err := userClients(ctx, users) // refuses a missing token
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pods, err := clients.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultStructuredOnly(pods), nil
		})

	// The application reads with its own client, so it asks Kubernetes first
	// whether the user may read the same object.
	s.AddTool(mcp.NewTool("get_settings", mcp.WithString("namespace", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			namespace, err := request.RequireString("namespace")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			clients, err := userClients(ctx, users)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			allowed, err := k8s.ReviewAccess(ctx, clients.Clientset, authorizationv1.ResourceAttributes{
				Verb: "get", Version: "v1", Resource: "configmaps", Namespace: namespace, Name: "settings",
			})
			if err != nil || !allowed {
				return mcp.NewToolResultError("not allowed"), nil
			}
			settings, err := application.CoreV1().ConfigMaps(namespace).Get(ctx, "settings", metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			logger.InfoContext(ctx, "settings read", slog.Any("user", user(ctx)))
			return mcp.NewToolResultStructuredOnly(settings.Data), nil
		})
	return s, nil
}
```

`list_pods` sends the separately obtained provider ID token, so Kubernetes enforces the caller's RBAC on the read.
`get_settings` reads with an application client, so it first asks Kubernetes with
`ReviewAccess` whether the user may read the same object, and fails closed on any review error.
Its log line names the user under `user`, using the same projection as the audit record. `ForToken`
refuses a missing token before any request is made. The module never falls back to the
application's credentials.
