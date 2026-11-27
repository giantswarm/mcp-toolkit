package k8s

import (
	"context"
	"fmt"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
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

// ReviewSubjectAccess evaluates a verified subject using the reviewing client's
// authority. The subject must come from authentication, such as TokenReview.
func ReviewSubjectAccess(ctx context.Context, client kubernetes.Interface, subject authenticationv1.UserInfo, attributes authorizationv1.ResourceAttributes) (bool, error) {
	if client == nil || strings.TrimSpace(subject.Username) == "" || subject.Username == "system:anonymous" {
		return false, fmt.Errorf("reviewing Kubernetes client and authenticated subject are required")
	}
	extra := make(map[string]authorizationv1.ExtraValue, len(subject.Extra))
	for key, values := range subject.Extra {
		extra[key] = authorizationv1.ExtraValue(values)
	}
	review, err := client.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User: subject.Username, UID: subject.UID, Groups: subject.Groups, Extra: extra, ResourceAttributes: &attributes,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return false, fmt.Errorf("subject access review: %w", err)
	}
	if review == nil || review.Status.EvaluationError != "" {
		return false, fmt.Errorf("subject access review could not be evaluated")
	}
	return review.Status.Allowed && !review.Status.Denied, nil
}
