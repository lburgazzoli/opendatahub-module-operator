# Include local overrides if present (gitignored).
-include local.mk

# Image URL to use all building/pushing image targets
IMG ?= ttl.sh/opendatahub-orchestrator-$(shell git rev-parse --short HEAD 2>/dev/null || echo dev):1h
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

MODULE_NAME ?= $(notdir $(CURDIR))

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

.PHONY: test-integration
test-integration: manifests generate install ## Run integration tests against the current cluster.
	go test -p 1 -v -tags=integration -count=1 -timeout=150s ./test/integration/...

.PHONY: lint
lint: ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run --build-tags integration,e2e

.PHONY: lint-fix
lint-fix: ## Run golangci-lint linter and perform fixes.
	$(GOLANGCI_LINT) run --fix

##@ Build

VERSION_PKG    = github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/version
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
container-prep: manifests generate ## Regenerate CRDs/RBAC and DeepCopy on host before container-build.

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	ODH_MODULE_OPERATOR_CHARTS_PATH=config/charts go run -ldflags "$(LDFLAGS)" ./cmd/main.go operator

.PHONY: container-build
container-build: container-prep ## Build container image with the manager.
	$(CONTAINER_TOOL) build -f Containerfile --build-arg LDFLAGS="$(LDFLAGS)" -t ${IMG} .

.PHONY: container-push
container-push: ## Push container image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

##@ Helm

HELM_NAMESPACE ?= opendatahub-orchestrator-system
HELM_RELEASE   ?= opendatahub-orchestrator
OPERATOR_NAMESPACE ?= $(HELM_NAMESPACE)
HELM_EXTRA_ARGS ?=

.PHONY: helm
helm: manifests generate ## Generate Helm chart from kustomize output.
	rm -rf config/chart
	$(KUSTOMIZE) build config/default | go run ./cmd/main.go chartgen --output config/chart

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: deploy-helm
deploy-helm: ## Deploy controller via Helm chart.
	@resolved_img="$$(bash ./hack/scripts/resolve-image-ref.sh "$(IMG)")"; \
		echo "Deploying image: $$resolved_img"; \
		$(HELM) upgrade --install $(HELM_RELEASE) config/chart \
			--namespace $(HELM_NAMESPACE) --create-namespace \
			--set-string image.fullRef="$$resolved_img" \
			--wait --timeout 5m $(HELM_EXTRA_ARGS)

.PHONY: push-openshift-image
push-openshift-image: ## Push a built image to the OpenShift internal registry and print the pullspec.
	@bash ./hack/scripts/push-openshift-image.sh "$(IMG)" "$(HELM_NAMESPACE)" "$(MODULE_NAME)"

.PHONY: deploy-openshift
deploy-openshift: ## Build locally, push to OpenShift internal registry, and deploy via Helm.
	$(MAKE) container-build
	@img="$$(bash ./hack/scripts/push-openshift-image.sh "$(IMG)" "$(HELM_NAMESPACE)" "$(MODULE_NAME)")"; \
		echo "Using image: $$img"; \
		$(MAKE) helm; \
		$(MAKE) deploy-helm IMG="$$img"

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
