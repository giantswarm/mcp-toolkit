# Kubernetes caller clients

`github.com/giantswarm/mcp-toolkit/k8s` is an independent Go module. It uses
client-go's `rest.AnonymousClientConfig` to retain endpoint, trust, proxy,
dial, throttling and timeout settings while removing base credentials.
Root toolkit consumers do not acquire client-go dependencies.

`NewUserClients` accepts a base REST configuration and an optional transport
wrapper. The wrapper must be credential-free, such as a diagnostic or tracing
wrapper. An opaque base transport or wrapper may contain credentials, so
client-go drops those; pass the approved wrapper explicitly.

`ForToken` requires a nonempty caller token for typed, dynamic and discovery
clients. Its bounded cache hashes token keys and evicts entries at token expiry.
Expiry parsing controls caching only. Expired tokens still reach Kubernetes
unchanged, where authentication is enforced. Application clients are selected
explicitly by the consumer and are outside this module.

`ReviewAccess` asks Kubernetes for a SelfSubjectAccessReview through the
supplied caller client. It returns false for denied or contradictory decisions
and errors for failed or unevaluable reviews. Consumers filter application-owned
reads on this result and retain their own public error representations.

Run checks independently with `GOWORK=off go test -race ./...`. The tests in
`envtest_test.go` run against a real kube-apiserver with RBAC when
`KUBEBUILDER_ASSETS` points at envtest binaries, and skip with `-short` or
without it. Source releases use `k8s/vX.Y.Z` tags through devctl's generated
nested-module workflow.
