package k8s

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	envtestNamespace   = "callers"
	readerName         = "reader"
	nobodyName         = "nobody"
	verbGet            = "get"
	configMaps         = "configmaps"
	configMapName      = "settings"
	undeclaredGroup    = "example.invalid"
	undeclaredResource = "widgets"
)

// apiServer is a real kube-apiserver with RBAC authorization and
// service-account token signing. It is nil when envtest cannot run.
var apiServer *envtestCluster

type envtestCluster struct {
	admin       *rest.Config
	readerToken string
	nobodyToken string
}

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() || os.Getenv("KUBEBUILDER_ASSETS") == "" {
		os.Exit(m.Run())
	}
	environment := &envtest.Environment{}
	admin, err := environment.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start envtest: %v\n", err)
		os.Exit(1)
	}
	apiServer, err = provisionCallers(context.Background(), admin)
	code := 1
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision envtest callers: %v\n", err)
	} else {
		code = m.Run()
	}
	if err := environment.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "stop envtest: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func requireAPIServer(t *testing.T) *envtestCluster {
	t.Helper()
	if apiServer == nil {
		t.Skip("envtest needs KUBEBUILDER_ASSETS and does not run with -short")
	}
	return apiServer
}

// provisionCallers creates two ServiceAccounts and returns TokenRequest tokens
// for them. reader may get ConfigMaps in envtestNamespace and widgets in
// undeclaredGroup, which has no CRD. nobody has no grants.
func provisionCallers(ctx context.Context, admin *rest.Config) (*envtestCluster, error) {
	client, err := kubernetes.NewForConfig(admin)
	if err != nil {
		return nil, err
	}
	if _, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: envtestNamespace}}, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	if _, err := client.CoreV1().ConfigMaps(envtestNamespace).Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: configMapName}}, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	reader := rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: readerName, Namespace: envtestNamespace}
	if _, err := client.RbacV1().Roles(envtestNamespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: readerName},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{verbGet}, APIGroups: []string{""}, Resources: []string{configMaps}}},
	}, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	if _, err := client.RbacV1().RoleBindings(envtestNamespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: readerName},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: readerName},
		Subjects:   []rbacv1.Subject{reader},
	}, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	if _, err := client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: readerName},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{verbGet}, APIGroups: []string{undeclaredGroup}, Resources: []string{undeclaredResource}}},
	}, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	if _, err := client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: readerName},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: readerName},
		Subjects:   []rbacv1.Subject{reader},
	}, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	cluster := &envtestCluster{admin: admin}
	for name, token := range map[string]*string{readerName: &cluster.readerToken, nobodyName: &cluster.nobodyToken} {
		if _, err := client.CoreV1().ServiceAccounts(envtestNamespace).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{}); err != nil {
			return nil, err
		}
		expiration := int64(time.Hour / time.Second)
		request, err := client.CoreV1().ServiceAccounts(envtestNamespace).CreateToken(ctx, name, &authenticationv1.TokenRequest{
			Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &expiration},
		}, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
		*token = request.Status.Token
	}
	return cluster, waitForGrants(ctx, admin, cluster.readerToken)
}

// waitForGrants waits until the RBAC authorizer's informers observe the
// reader bindings, so tests do not race policy propagation.
func waitForGrants(ctx context.Context, admin *rest.Config, token string) error {
	users, err := NewUserClients(admin, nil)
	if err != nil {
		return err
	}
	clients, err := users.ForToken(token)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		namespaced, err := ReviewAccess(ctx, clients.Clientset, configMapAccess())
		if err != nil {
			return err
		}
		clusterWide, err := ReviewAccess(ctx, clients.Clientset, widgetAccess())
		if err != nil {
			return err
		}
		if namespaced && clusterWide {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("reader grants not observed by the RBAC authorizer")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func configMapAccess() authorizationv1.ResourceAttributes {
	return authorizationv1.ResourceAttributes{Verb: verbGet, Version: "v1", Resource: configMaps, Namespace: envtestNamespace, Name: configMapName}
}

func widgetAccess() authorizationv1.ResourceAttributes {
	return authorizationv1.ResourceAttributes{Verb: verbGet, Group: undeclaredGroup, Version: "v1", Resource: undeclaredResource, Name: "anything"}
}

func callerClients(t *testing.T, cluster *envtestCluster, token string) *Clients {
	t.Helper()
	users, err := NewUserClients(cluster.admin, nil)
	require.NoError(t, err)
	clients, err := users.ForToken(token)
	require.NoError(t, err)
	return clients
}

func TestEnvtestCallerTokenReachesAPIAsCaller(t *testing.T) {
	cluster := requireAPIServer(t)
	require.NotEmpty(t, cluster.admin.CertData, "admin must authenticate with a client certificate the caller client drops")

	clients := callerClients(t, cluster, cluster.readerToken)

	review, err := clients.Clientset.AuthenticationV1().SelfSubjectReviews().Create(t.Context(), &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.Equal(t, "system:serviceaccount:"+envtestNamespace+":"+readerName, review.Status.UserInfo.Username)

	configMap, err := clients.Clientset.CoreV1().ConfigMaps(envtestNamespace).Get(t.Context(), configMapName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, configMapName, configMap.Name)

	_, err = clients.Clientset.CoreV1().Secrets(envtestNamespace).List(t.Context(), metav1.ListOptions{})
	require.True(t, apierrors.IsForbidden(err), "reader has no Secret grant: %v", err)
}

func TestEnvtestCallerWithoutGrantIsForbidden(t *testing.T) {
	cluster := requireAPIServer(t)
	clients := callerClients(t, cluster, cluster.nobodyToken)

	_, err := clients.Clientset.CoreV1().ConfigMaps(envtestNamespace).Get(t.Context(), configMapName, metav1.GetOptions{})
	require.True(t, apierrors.IsForbidden(err), "nobody has no ConfigMap grant: %v", err)
}

func TestEnvtestMissingTokenIsRefusedBeforeAnyRequest(t *testing.T) {
	cluster := requireAPIServer(t)
	var requests atomic.Int32
	users, err := NewUserClients(cluster.admin, func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			requests.Add(1)
			return next.RoundTrip(request)
		})
	})
	require.NoError(t, err)

	for _, token := range []string{"", " \t"} {
		clients, err := users.ForToken(token)
		require.Error(t, err)
		require.Nil(t, clients)
	}
	require.Zero(t, requests.Load())

	clients, err := users.ForToken(cluster.readerToken)
	require.NoError(t, err)
	_, err = clients.Clientset.CoreV1().ConfigMaps(envtestNamespace).Get(t.Context(), configMapName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Positive(t, requests.Load(), "the counting wrapper must observe caller requests")
}

func TestEnvtestReviewAccess(t *testing.T) {
	cluster := requireAPIServer(t)
	for _, test := range []struct {
		name       string
		token      string
		attributes authorizationv1.ResourceAttributes
		allowed    bool
	}{
		{name: "granted", token: cluster.readerToken, attributes: configMapAccess(), allowed: true},
		{name: "not granted", token: cluster.nobodyToken, attributes: configMapAccess()},
		{name: "granted resource without CRD", token: cluster.readerToken, attributes: widgetAccess(), allowed: true},
		{name: "not granted resource without CRD", token: cluster.nobodyToken, attributes: widgetAccess()},
	} {
		t.Run(test.name, func(t *testing.T) {
			clients := callerClients(t, cluster, test.token)
			allowed, err := ReviewAccess(t.Context(), clients.Clientset, test.attributes)
			require.NoError(t, err)
			require.Equal(t, test.allowed, allowed)
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
