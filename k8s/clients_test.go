package k8s

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type testTransport func(*http.Request) (*http.Response, error)

const (
	baseCredential      = "service-account"
	callerAuthorization = "Bearer caller-token"
)

func (transport testTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestOnlyCallerCredentialReachesAPI(t *testing.T) {
	var lock sync.Mutex
	var authorizations []string
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		lock.Lock()
		authorizations = append(authorizations, request.Header.Get("Authorization"))
		lock.Unlock()
		assert.Empty(t, request.Header.Get("Impersonate-User"))
		assert.Equal(t, "kept", request.Header.Get("X-Transport-Policy"))
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1" {
			_, _ = writer.Write([]byte(`{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","namespaced":true,"kind":"ConfigMap","verbs":["list"]}]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"kind":"ConfigMapList","apiVersion":"v1","items":[]}`))
	}))
	t.Cleanup(api.Close)
	base := &rest.Config{
		Host: api.URL, Username: "base-user", Password: "base-password", BearerToken: baseCredential,
		BearerTokenFile: filepath.Join(t.TempDir(), "must-not-read"),
		Impersonate:     rest.ImpersonationConfig{UserName: "system:serviceaccount:default:manager"},
		TLSClientConfig: rest.TLSClientConfig{CertData: []byte("base-client-cert"), KeyData: []byte("base-client-key")},
		QPS:             100, Burst: 200, Timeout: 5 * time.Second, UserAgent: "manager",
		Transport: testTransport(func(*http.Request) (*http.Response, error) { panic("base transport must not be used") }),
	}
	cache, err := NewUserClients(base, func(next http.RoundTripper) http.RoundTripper {
		return testTransport(func(request *http.Request) (*http.Response, error) {
			request.Header.Set("X-Transport-Policy", "kept")
			return next.RoundTrip(request)
		})
	})
	require.NoError(t, err)
	require.Equal(t, base.QPS, cache.configuration.QPS)
	require.Equal(t, base.Burst, cache.configuration.Burst)
	require.Equal(t, base.Timeout, cache.configuration.Timeout)
	require.Empty(t, cache.configuration.CertData)
	require.Empty(t, cache.configuration.KeyData)
	require.Empty(t, cache.configuration.BearerTokenFile)
	require.Empty(t, cache.configuration.Username)
	require.Empty(t, cache.configuration.Password)
	clients, err := cache.ForToken("caller-token")
	require.NoError(t, err)
	_, err = clients.Clientset.CoreV1().ConfigMaps("default").List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	_, err = clients.Dynamic.Resource(schema.GroupVersionResource{Version: "v1", Resource: configMaps}).Namespace("default").List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	_, err = clients.Discovery.ServerResourcesForGroupVersion("v1")
	require.NoError(t, err)
	lock.Lock()
	defer lock.Unlock()
	require.Equal(t, []string{callerAuthorization, callerAuthorization, callerAuthorization}, authorizations)
}

func TestConstructionFailureAndMissingTokenMakeNoRequest(t *testing.T) {
	var requests atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	t.Cleanup(api.Close)
	cache, err := NewUserClients(&rest.Config{Host: api.URL, TLSClientConfig: rest.TLSClientConfig{CAFile: filepath.Join(t.TempDir(), "missing-ca")}}, nil)
	require.NoError(t, err)
	for _, token := range []string{"", " ", "caller-token"} {
		clients, err := cache.ForToken(token)
		require.Error(t, err)
		require.Nil(t, clients)
	}
	require.Zero(t, requests.Load())
	require.Empty(t, cache.cache)
}

func TestExpiredCredentialRemainsCallerBound(t *testing.T) {
	token := expiringToken(t, time.Now().Add(-time.Minute))
	var authorization string
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Unauthorized","code":401,"message":"expired caller credential"}`))
	}))
	t.Cleanup(api.Close)
	cache, err := NewUserClients(&rest.Config{Host: api.URL, BearerToken: baseCredential}, nil)
	require.NoError(t, err)
	clients, err := cache.ForToken(token)
	require.NoError(t, err)
	_, err = clients.Clientset.CoreV1().ConfigMaps("default").List(t.Context(), metav1.ListOptions{})
	require.True(t, apierrors.IsUnauthorized(err), "%v", err)
	require.Equal(t, "Bearer "+token, authorization)
	require.Empty(t, cache.cache)
}

func TestCacheEvictsAtTokenExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache, err := NewUserClients(&rest.Config{Host: "https://api.example.test"}, nil)
		require.NoError(t, err)
		token := expiringToken(t, time.Now().Add(time.Minute))
		first, err := cache.ForToken(token)
		require.NoError(t, err)
		cached, err := cache.ForToken(token)
		require.NoError(t, err)
		require.Same(t, first, cached)
		time.Sleep(time.Minute)
		expired, err := cache.ForToken(token)
		require.NoError(t, err)
		require.NotSame(t, cached, expired)
		require.Empty(t, cache.cache)
	})
}

func expiringToken(t *testing.T, expiry time.Time) string {
	t.Helper()
	payload, err := json.Marshal(map[string]int64{"exp": expiry.Unix()})
	require.NoError(t, err)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
