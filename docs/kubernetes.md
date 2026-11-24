# Kubernetes caller clients

The core packages and the optional [`k8s`](../k8s) module work together like this. The core
packages apply tool policy. The `k8s` module makes every Kubernetes call with the caller's own
token, so the caller's RBAC decides what the tool may read.

`k8s` is a separate Go module, so client-go stays out of servers that do not use it:

```sh
go get github.com/giantswarm/mcp-toolkit github.com/giantswarm/mcp-toolkit/k8s
```

The application's OAuth layer validates the caller and stores the identity and raw token on the
request context. `callerIdentity` and `callerToken` below stand for the application's own accessors.

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

	"github.com/giantswarm/mcp-toolkit/k8s"
	policy "github.com/giantswarm/mcp-toolkit/middleware/mcpgo"
	"github.com/giantswarm/mcp-toolkit/toolpolicy"
)

// The application's OAuth layer validates the caller and stores the identity
// and raw token on the request context.
var (
	callerIdentity func(context.Context) (toolpolicy.Identity, bool)
	callerToken    func(context.Context) string
)

func newServer(base *rest.Config, application kubernetes.Interface, logger *slog.Logger) (*mcpserver.MCPServer, error) {
	users, err := k8s.NewUserClients(base, nil)
	if err != nil {
		return nil, err
	}
	s := mcpserver.NewMCPServer("my-mcp", "1.0.0",
		policy.New(toolpolicy.Options{MaxResultBytes: 128 << 10, Logger: logger, User: callerIdentity}),
	)

	// The caller's own RBAC governs the read.
	s.AddTool(mcp.NewTool("list_pods", mcp.WithString("namespace", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			namespace, err := request.RequireString("namespace")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			clients, err := users.ForToken(callerToken(ctx)) // refuses a missing token
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
	// whether the caller may read the same object.
	s.AddTool(mcp.NewTool("get_settings", mcp.WithString("namespace", mcp.Required())),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			namespace, err := request.RequireString("namespace")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			clients, err := users.ForToken(callerToken(ctx))
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
			return mcp.NewToolResultStructuredOnly(settings.Data), nil
		})
	return s, nil
}
```

`list_pods` sends the caller's token, so Kubernetes enforces the caller's RBAC on the read.
`get_settings` reads with an application client, so it first asks Kubernetes with
`ReviewAccess` whether the caller may read the same object, and fails closed on any review error.
`ForToken` refuses a missing token before any request is made. The module never falls back to the
application's credentials.
