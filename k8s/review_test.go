package k8s

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
