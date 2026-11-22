package k8s

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ReviewAccess asks the API to evaluate the client's own identity. Missing
// clients and unevaluable or contradictory reviews fail closed.
func ReviewAccess(ctx context.Context, client kubernetes.Interface, attributes authorizationv1.ResourceAttributes) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("caller Kubernetes client is required")
	}
	review, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &attributes},
	}, metav1.CreateOptions{})
	if err != nil {
		return false, fmt.Errorf("self-subject access review: %w", err)
	}
	if review == nil || review.Status.EvaluationError != "" {
		return false, fmt.Errorf("self-subject access review could not be evaluated")
	}
	return review.Status.Allowed && !review.Status.Denied, nil
}
