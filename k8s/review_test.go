package k8s

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
)

func TestAccessReviewUsesCallerAndFailsClosed(t *testing.T) {
	const (
		apiError              = "API error"
		allowedReview         = "allowed"
		contradictoryReview   = "contradictory"
		evaluationErrorReview = "evaluation error"
	)
	for _, name := range []string{allowedReview, "denied", evaluationErrorReview, contradictoryReview, apiError} {
		t.Run(name, func(t *testing.T) {
			attributes := authorizationv1.ResourceAttributes{Verb: verbGet, Group: "", Version: "v1", Resource: configMaps, Namespace: "default", Name: "configuration"}
			api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assert.Equal(t, "Bearer caller-token", request.Header.Get("Authorization"))
				assert.Equal(t, "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews", request.URL.Path)
				var review authorizationv1.SelfSubjectAccessReview
				body, err := io.ReadAll(request.Body)
				if !assert.NoError(t, err) {
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				_, _, err = scheme.Codecs.UniversalDeserializer().Decode(body, nil, &review)
				if !assert.NoError(t, err) {
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				assert.Equal(t, &attributes, review.Spec.ResourceAttributes)
				if name == apiError {
					writer.WriteHeader(http.StatusForbidden)
					return
				}
				review.Status.Allowed = name == allowedReview || name == evaluationErrorReview || name == contradictoryReview
				review.Status.Denied = name == "denied" || name == contradictoryReview
				if name == evaluationErrorReview {
					review.Status.EvaluationError = "policy unavailable"
				}
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(review)
			}))
			t.Cleanup(api.Close)
			cache, err := NewUserClients(&rest.Config{Host: api.URL, BearerToken: baseCredential}, nil)
			require.NoError(t, err)
			clients, err := cache.ForToken("caller-token")
			require.NoError(t, err)
			allowed, err := ReviewAccess(t.Context(), clients.Clientset, attributes)
			if name == apiError || name == evaluationErrorReview {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, name == allowedReview, allowed)
		})
	}
	allowed, err := ReviewAccess(t.Context(), nil, authorizationv1.ResourceAttributes{})
	require.Error(t, err)
	require.False(t, allowed)
}

func TestReviewSubjectAccess(t *testing.T) {
	subject := authenticationv1.UserInfo{Username: "system:serviceaccount:managers:models", UID: "account-uid", Groups: []string{"system:authenticated", "system:serviceaccounts"}, Extra: map[string]authenticationv1.ExtraValue{"example.org/actor": {"actor"}}}
	attributes := authorizationv1.ResourceAttributes{Namespace: "credentials", Verb: "create", Group: "credential.giantswarm.io", Resource: "operations"}
	for _, test := range []struct {
		name      string
		status    authorizationv1.SubjectAccessReviewStatus
		want      bool
		wantError bool
	}{
		{name: "allow", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}, want: true},
		{name: "deny", status: authorizationv1.SubjectAccessReviewStatus{Denied: true}},
		{name: "no decision"},
		{name: "contradictory", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true, Denied: true}},
		{name: "evaluation failed", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true, EvaluationError: "unavailable"}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewClientset()
			client.PrependReactor("create", "subjectaccessreviews", func(action clienttesting.Action) (bool, runtime.Object, error) {
				review := action.(clienttesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview)
				require.Equal(t, subject.Username, review.Spec.User)
				require.Equal(t, subject.UID, review.Spec.UID)
				require.Equal(t, subject.Groups, review.Spec.Groups)
				require.Equal(t, authorizationv1.ExtraValue{"actor"}, review.Spec.Extra["example.org/actor"])
				require.Equal(t, &attributes, review.Spec.ResourceAttributes)
				return true, &authorizationv1.SubjectAccessReview{Status: test.status}, nil
			})
			allowed, err := ReviewSubjectAccess(t.Context(), client, subject, attributes)
			require.Equal(t, test.wantError, err != nil)
			require.Equal(t, test.want, allowed)
		})
	}
	for _, subject := range []authenticationv1.UserInfo{{}, {Username: " "}, {Username: "system:anonymous"}} {
		client := fake.NewClientset()
		allowed, err := ReviewSubjectAccess(t.Context(), client, subject, attributes)
		require.Error(t, err)
		require.False(t, allowed)
		require.Empty(t, client.Actions())
	}
	allowed, err := ReviewSubjectAccess(t.Context(), nil, subject, attributes)
	require.Error(t, err)
	require.False(t, allowed)
}
