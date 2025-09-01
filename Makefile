# Image URL to use for building/pushing image targets
MANAGER_IMG ?= manager:latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary
ENVTEST_K8S_VERSION = 1.29.0

# Manager deployment configuration
MANAGER_NAMESPACE ?= namespacelabel-system
MANAGER_DEPLOYMENT ?= namespacelabel-controller-manager
DEPLOYMENT_TIMEOUT ?= 300s

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Code Generation

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

##@ Code Quality

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

##@ Testing

.PHONY: test
test: manifests generate fmt vet envtest ginkgo ## Run unit tests. Use GINKGO_FOCUS=label to run specific tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) -v --procs=16 --compilers=16 --show-node-events --coverprofile cover.out $(if $(GINKGO_FOCUS),--label-filter="$(GINKGO_FOCUS)") ./internal/controller/ ./api/...
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) -v --procs=1 --compilers=1 --show-node-events $(if $(GINKGO_FOCUS),--label-filter="$(GINKGO_FOCUS)") ./internal/webhook/...

# GINKGO_FOCUS can be set to run specific tests by label
# Examples:
#   make test GINKGO_FOCUS=controller     # Run only controller unit tests
#   make test GINKGO_FOCUS=webhook        # Run only webhook unit tests
#   make test-e2e GINKGO_FOCUS=controller # Run only controller e2e tests  
#   make test-e2e GINKGO_FOCUS=webhook    # Run only webhook e2e tests
GINKGO_FOCUS ?=

.PHONY: test-e2e
test-e2e: ginkgo ## Run E2E tests in parallel. Use GINKGO_FOCUS=label to run specific tests.
	@echo "Running e2e tests against cluster..."
	@echo "Checking cluster connectivity..."
	@(kubectl get ns > /dev/null 2>&1) || (echo "ERROR: Cannot connect to cluster. Ensure cluster is running and kubeconfig is correct." && exit 1)
	$(GINKGO) -v --procs=16 --compilers=16 --fail-on-pending --show-node-events --timeout=15m $(if $(GINKGO_FOCUS),--label-filter="$(GINKGO_FOCUS)") ./test/e2e/

.PHONY: test-e2e-debug
test-e2e-debug: ## Run E2E tests sequentially for debugging. Use GINKGO_FOCUS=label to run specific tests.
	go test ./test/e2e/ -v -timeout 15m --ginkgo.v --ginkgo.fail-on-pending $(if $(GINKGO_FOCUS),--ginkgo.label-filter="$(GINKGO_FOCUS)")

##@ Development

.PHONY: build
build: generate ## Build manager binary.
	go build -o bin/manager cmd/manager/main.go

.PHONY: run
run: generate ## Run the manager (controller + webhook) from your host.
	go run ./cmd/manager/main.go

##@ Build

.PHONY: docker-build
docker-build: ## Build docker image with the manager (controller + webhook).
	$(CONTAINER_TOOL) build -t ${MANAGER_IMG} -f cmd/manager/Dockerfile .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${MANAGER_IMG}

.PHONY: generate-installer
generate-installer: manifests kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	@cd config/manager && $(KUSTOMIZE) edit set image manager=${MANAGER_IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = true
endif

.PHONY: cert-manager-install
cert-manager-install: ## Install cert-manager (required for webhook certificates)
	@kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
	@kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=300s
	@kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=300s
	@kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=300s

.PHONY: cert-manager-uninstall
cert-manager-uninstall: ## Uninstall cert-manager from the K8s cluster
	@kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml --ignore-not-found=true

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy the complete operator (controller and webhook) to the K8s cluster.
	@cd config/manager && $(KUSTOMIZE) edit set image manager=${MANAGER_IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -
	@kubectl wait --for=condition=Ready certificate/namespacelabel-webhook-serving-cert -n namespacelabel-system --timeout=300s || echo "⚠️  Certificate may still be provisioning"

.PHONY: undeploy
undeploy: kustomize ## Undeploy the complete operator from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -
	@cd config/manager && $(KUSTOMIZE) edit set image manager=manager:latest

##@ Monitoring

.PHONY: deploy-status
deploy-status: ## Show detailed deployment status.
	@echo "📊 Deployment Status for $(MANAGER_IMG):"
	@echo ""
	@echo "🏗️  Manager Deployment:"
	@$(KUBECTL) get deployment $(MANAGER_DEPLOYMENT) -n $(MANAGER_NAMESPACE) -o wide 2>/dev/null || echo "❌ Manager not deployed"
	@echo ""
	@echo "🚀 All Pods:"
	@$(KUBECTL) get pods -n $(MANAGER_NAMESPACE) -o wide 2>/dev/null || echo "❌ No pods found"
	@echo ""
	@echo "📋 Recent Events:"
	@$(KUBECTL) get events -n $(MANAGER_NAMESPACE) --sort-by='.lastTimestamp' | tail -10 2>/dev/null || echo "❌ No events found"

##@ Dependencies

# Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GINKGO ?= $(LOCALBIN)/ginkgo-$(GINKGO_VERSION)

# Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.14.0
ENVTEST_VERSION ?= release-0.17
GOLANGCI_LINT_VERSION ?= v1.61.0
GINKGO_VERSION ?= v2.15.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo locally if necessary.
$(GINKGO): $(LOCALBIN)
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo,$(GINKGO_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef
