# Include local overrides if present (gitignored).
-include local.mk

# Root Makefile is monorepo orchestration only.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
CONTAINER_TOOL ?= podman
ALL_MODULES_IMG ?= ttl.sh/opendatahub-module-operators-$(shell git rev-parse --short HEAD 2>/dev/null || echo dev):1h
VERSION ?= 0.0.0-dev
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
GIT_REPO ?= $(shell git remote get-url origin 2>/dev/null || echo unknown)

MODULE_DIRS ?= \
	modules/opendatahub-datasciencepipelines-operator \
	modules/opendatahub-feast-operator \
	modules/opendatahub-mlflow-operator \
	modules/opendatahub-modelregistry-operator \
	modules/opendatahub-ogx-operator \
	modules/opendatahub-ray-operator \
	modules/opendatahub-spark-operator \
	modules/opendatahub-trainer-operator \
	modules/opendatahub-trustyai-operator \
	modules/opendatahub-workbenches-operator \
	modules/opendatahub-db-operator

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

define run_in_modules
	@set -euo pipefail; \
	for module in $(MODULE_DIRS); do \
		echo "==> $$module :: $(1)"; \
		$(MAKE) -C "$$module" $(1); \
	done
endef

##@ Monorepo

.PHONY: list-modules
list-modules: ## Print the module directories included by aggregate targets.
	@printf '%s\n' $(MODULE_DIRS)

.PHONY: test-modules
test-modules: ## Run unit-test workflows across all tracked module operators.
	$(call run_in_modules,test)

.PHONY: lint-modules
lint-modules: ## Run golangci-lint across all tracked module operators.
	$(call run_in_modules,lint)

.PHONY: verify-all
verify-all: test-modules lint-modules ## Run the standard aggregate verification suite.

.PHONY: container-build-all-modules
container-build-all-modules: ## Build a single image containing all real module operators.
	$(CONTAINER_TOOL) build -f Containerfile \
		--build-arg VERSION="$(VERSION)" \
		--build-arg GIT_COMMIT="$(GIT_COMMIT)" \
		--build-arg GIT_BRANCH="$(GIT_BRANCH)" \
		--build-arg GIT_REPO="$(GIT_REPO)" \
		-t ${ALL_MODULES_IMG} .
