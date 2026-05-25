# Include local overrides if present (gitignored).
-include local.mk

# Root Makefile is monorepo orchestration only.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

MODULE_DIRS ?= \
	modules/opendatahub-feast-operator \
	modules/opendatahub-mlflow-operator \
	modules/opendatahub-modelregistry-operator \
	modules/opendatahub-mymodule-operator \
	modules/opendatahub-ogx-operator \
	modules/opendatahub-ray-operator \
	modules/opendatahub-spark-operator \
	modules/opendatahub-trustyai-operator

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
