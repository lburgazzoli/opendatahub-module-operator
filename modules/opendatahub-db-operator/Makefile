# Include local overrides if present (gitignored).
-include local.mk

# Image URL to use all building/pushing image targets
IMG ?= quay.io/lburgazzoli/opendatahub-db-operator:dev
# YEAR defines the year value used for substituting the YEAR placeholder in the boilerplate header.
YEAR ?= $(shell date +%Y)

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= podman

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

## Tool Versions
CONTROLLER_GEN_VERSION ?= v0.20.1
KUSTOMIZE_VERSION      ?= v5.8.1
GOLANGCI_LINT_VERSION  ?= v2.11.4
HELM_VERSION           ?= v4.2.0

## Tool Commands
CONTROLLER_GEN = go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
KUSTOMIZE      = go run sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)
GOLANGCI_LINT  = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
HELM           = go run helm.sh/helm/v4/cmd/helm@$(HELM_VERSION)

KUBECTL ?= kubectl

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt",year=$(YEAR) paths="./..."

.PHONY: fmt
fmt: ## Run golangci-lint formatters against code.
	$(GOLANGCI_LINT) fmt

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: deps
deps: ## Tidy and verify Go module dependencies.
	go mod tidy
	go mod verify

.PHONY: test
test: manifests generate fmt vet ## Run unit tests.
	go test -ldflags "$(LDFLAGS)" $$(go list ./... | grep -v /e2e | grep -v /integration) -coverprofile cover.out

.PHONY: test-integration-run
test-integration-run: ## Run integration tests only (cluster must be prepared).
	INTEGRATION_TEST_NAMESPACE="$(INTEGRATION_TEST_NAMESPACE)" TEST_PLATFORM_VERSION="$(TEST_PLATFORM_VERSION)" go test -ldflags "$(LDFLAGS)" ./test/integration/ -tags=integration -v -timeout 5m -failfast

.PHONY: test-integration-setup
test-integration-setup: manifests generate ## Prepare the cluster for integration tests.
	$(MAKE) cleanup-integration
	$(MAKE) install

.PHONY: test-integration
test-integration: test-integration-setup test-integration-run ## Run integration tests (prepare cluster, then run tests).

.PHONY: test-e2e-run
test-e2e-run: ## Run e2e tests only (operator must already be deployed).
	OPERATOR_NAMESPACE="$(OPERATOR_NAMESPACE)" go test -ldflags "$(LDFLAGS)" ./test/e2e/ -tags=e2e -v -timeout 10m -failfast

.PHONY: test-e2e-setup
test-e2e-setup: ## Prepare the cluster for e2e tests.
	@:

.PHONY: test-e2e
test-e2e: HELM_EXTRA_ARGS += --set-string platform.type=$(TEST_PLATFORM_TYPE) --set-string platform.version=$(TEST_PLATFORM_VERSION)
test-e2e: test-e2e-cleanup test-e2e-setup deploy-helm test-e2e-run ## Run e2e tests (clean, prepare, deploy, then run).

.PHONY: test-integration-cleanup
test-integration-cleanup: ## Clean up integration test resources from the cluster.
	./hack/scripts/cleanup-integration.sh "$(INTEGRATION_TEST_NAMESPACE)"

.PHONY: cleanup-integration
cleanup-integration: test-integration-cleanup ## Alias for test-integration-cleanup.

.PHONY: test-e2e-cleanup
test-e2e-cleanup: ## Clean up e2e test resources and uninstall operator from the cluster.
	./hack/scripts/cleanup-e2e.sh "$(OPERATOR_NAMESPACE)" "$(HELM_RELEASE)"

.PHONY: cleanup-e2e
cleanup-e2e: test-e2e-cleanup ## Alias for test-e2e-cleanup.

.PHONY: lint
lint: ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run --build-tags integration,e2e,upgrade

.PHONY: lint-fix
lint-fix: ## Run golangci-lint linter and perform fixes.
	$(GOLANGCI_LINT) run --fix

##@ Build

VERSION_PKG    = github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/module
VERSION       ?= 0.0.0-dev
GIT_COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_BRANCH    ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
GIT_REPO      ?= $(shell git remote get-url origin 2>/dev/null || echo unknown)
GOOS          ?= $(shell go env GOOS)
GOARCH        ?= $(shell go env GOARCH)
CGO_ENABLED   ?= 0
BIN_DIR       ?= bin
BIN_NAME      ?= manager
LDFLAGS        = -X $(VERSION_PKG).Version=$(VERSION) \
                 -X $(VERSION_PKG).Commit=$(GIT_COMMIT) \
                 -X $(VERSION_PKG).Branch=$(GIT_BRANCH) \
                 -X $(VERSION_PKG).Repo=$(GIT_REPO)

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	mkdir -p "$(BIN_DIR)"
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(BIN_NAME)" cmd/main.go

.PHONY: build-bin
build-bin: ## Build manager binary only (for Containerfile; run container-prep on host first).
	mkdir -p "$(BIN_DIR)"
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/$(BIN_NAME)" cmd/main.go

.PHONY: container-prep
container-prep: manifests generate ## On host: regenerate code before container-build.

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	ODH_MODULE_OPERATOR_CONTROLLER_LEADER_ELECTION_ENABLED=false go run -ldflags "$(LDFLAGS)" ./cmd/main.go operator

.PHONY: container-build
container-build: container-prep ## Build container image with the manager.
	$(CONTAINER_TOOL) build -f Containerfile --build-arg LDFLAGS="$(LDFLAGS)" -t ${IMG} .

.PHONY: container-push
container-push: ## Push container image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: deploy-help
deploy-help: ## Show the supported image build and Helm deploy workflow.
	@printf '%s\n' \
		'Run deployment commands from modules/opendatahub-db-operator/.' \
		'' \
		'Typical workflow:' \
		'  make container-build IMG="$(IMG)"' \
		'  make container-push IMG="$(IMG)"' \
		'  make helm' \
		'  make deploy-helm IMG="$(IMG)"' \
		'' \
		'Useful overrides:' \
		'  HELM_NAMESPACE=<namespace>' \
		'  HELM_RELEASE=<release-name>' \
		'  TEST_PLATFORM_TYPE=<OpenDataHub|RHOAI>' \
		'  TEST_PLATFORM_VERSION=<version>'

##@ Helm

HELM_NAMESPACE ?= odh-db-operator-system
HELM_RELEASE   ?= odh-db-operator
OPERATOR_NAMESPACE ?= $(HELM_NAMESPACE)
INTEGRATION_TEST_NAMESPACE ?= odh-db-operator-integration
HELM_EXTRA_ARGS ?=
TEST_PLATFORM_TYPE ?= OpenDataHub
TEST_PLATFORM_VERSION ?= 0.1.0

.PHONY: helm
helm: manifests generate ## Generate Helm chart from kustomize output.
	rm -rf config/chart
	$(KUSTOMIZE) build config/default | go run ./cmd/main.go chartgen --output config/chart

.PHONY: helm-status
helm-status: ## Show Helm release status.
	$(HELM) status $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: deploy-helm
deploy-helm: ## Deploy controller via Helm chart.
	@echo "Deploying image: $(IMG)"; \
		$(HELM) upgrade --install $(HELM_RELEASE) config/chart \
			--namespace $(HELM_NAMESPACE) --create-namespace \
			--set-string operator.image.ref="$(IMG)" \
			--wait --timeout 5m $(HELM_EXTRA_ARGS)

.PHONY: undeploy-helm
undeploy-helm: ## Undeploy controller installed via Helm.
	-$(HELM) uninstall $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( $(KUSTOMIZE) build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | $(KUBECTL) apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	@out="$$( $(KUSTOMIZE) build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

##@ Helpers

.PHONY: kustomize
kustomize:

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
