# setup-envtest follows the k8s module's controller-runtime version, and the
# kube-apiserver follows its k8s.io/api minor, as in the kubebuilder scaffold.
ENVTEST_VERSION ?= $(shell cd k8s && GOWORK=off go list -m -f '{{.Version}}' sigs.k8s.io/controller-runtime)
ENVTEST_K8S_VERSION ?= $(shell cd k8s && GOWORK=off go list -m -f '{{.Version}}' k8s.io/api | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

.PHONY: test-envtest
test-envtest: ## Run the k8s module tests against a real kube-apiserver with RBAC (envtest).
	@set -eu; \
	assets="$$(GOWORK=off go run sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION) use $(ENVTEST_K8S_VERSION) -p path)"; \
	test -n "$$assets"; \
	cd k8s && KUBEBUILDER_ASSETS="$$assets" GOWORK=off go test -race -count=1 -run Envtest -v ./...
