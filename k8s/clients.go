// Package k8s constructs caller-bound Kubernetes clients and access reviews.
package k8s

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
)

// Clients uses one credential across typed, dynamic and discovery operations.
type Clients struct {
	Clientset kubernetes.Interface
	Dynamic   dynamic.Interface
	Discovery discovery.DiscoveryInterface
}

type cachedClients struct {
	clients *Clients
	expires time.Time
}

const maxCachedClients = 256

// UserClients caches clients by caller token. It has no application-credential
// mode. Connection settings follow client-go's AnonymousClientConfig policy.
type UserClients struct {
	configuration *rest.Config
	mu            sync.Mutex
	cache         map[[32]byte]cachedClients
}

// NewUserClients removes base credentials. wrapper explicitly augments the
// caller's transport and must not inject credentials of its own.
func NewUserClients(base *rest.Config, wrapper transport.WrapperFunc) (*UserClients, error) {
	if base == nil || base.Host == "" {
		return nil, fmt.Errorf("kubernetes endpoint configuration is required")
	}
	configuration := rest.AnonymousClientConfig(base)
	configuration.WrapTransport = wrapper
	return &UserClients{configuration: configuration, cache: make(map[[32]byte]cachedClients)}, nil
}

// ForToken requires a caller credential, including after expiry. Expired tokens
// reach the API unchanged, are never cached, and never acquire other authority.
func (c *UserClients) ForToken(token string) (*Clients, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("caller token is required")
	}
	key := sha256.Sum256([]byte(token))
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, entry := range c.cache {
		if !entry.expires.IsZero() && !now.Before(entry.expires) {
			delete(c.cache, key)
		}
	}
	if cached, ok := c.cache[key]; ok {
		return cached.clients, nil
	}
	configuration := rest.CopyConfig(c.configuration)
	configuration.BearerToken = token
	dynamicClient, err := dynamic.NewForConfig(configuration)
	if err != nil {
		return nil, fmt.Errorf("caller dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(configuration)
	if err != nil {
		return nil, fmt.Errorf("caller discovery client: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(configuration)
	if err != nil {
		return nil, fmt.Errorf("caller clientset: %w", err)
	}
	clients := &Clients{Clientset: clientset, Dynamic: dynamicClient, Discovery: discoveryClient}
	expires := tokenExpiry(token)
	if !expires.IsZero() && !now.Before(expires) {
		return clients, nil
	}
	if len(c.cache) >= maxCachedClients {
		clear(c.cache)
	}
	c.cache[key] = cachedClients{clients: clients, expires: expires}
	return clients, nil
}

func tokenExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Expiry int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Expiry == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Expiry, 0)
}
