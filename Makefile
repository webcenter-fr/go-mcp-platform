# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: check-components
check-components:
	bash scripts/check_components.sh

.PHONY: ui
ui: ## Build the approval UI into internal/approval/ui/dist (committed for go:embed).
	cd internal/approval/ui && npm ci && npm run build

.PHONY: test
test:
	go test -v ./...

.PHONY: docker-build-mcp
docker-build-mcp: ## Build the MCP server image.
	docker build -f Dockerfile.mcp -t go-mcp-platform-mcp:dev .

.PHONY: docker-build-approval
docker-build-approval: ## Build the approval server image.
	docker build -f Dockerfile.approval -t go-mcp-platform-approval:dev .

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	helm lint helm/go-mcp-platform

.PHONY: helm-template
helm-template: ## Render the chart and every fixture; assert the guards.
	helm template t helm/go-mcp-platform > /dev/null
	for f in helm/go-mcp-platform/tests/values-*.yaml; do \
		case "$$(basename "$$f" .yaml)" in \
			values-memory-multi-replica|values-file-no-persistence|values-oidc-no-secret) \
				if helm template t helm/go-mcp-platform -f "$$f" > /dev/null 2>&1; then \
					echo "GUARD BROKEN: $$f must fail"; exit 1; \
				fi; \
				;; \
			*) \
				helm template t helm/go-mcp-platform -f "$$f" > /dev/null; \
				;; \
		esac; \
	done

.PHONY: helm-schema
helm-schema: ## Regenerate values.schema.json.
	dagger call helm-schema export --path helm/go-mcp-platform/values.schema.json

.PHONY: helm-docs
helm-docs: ## Regenerate the chart README.
	dagger call helm-docs export --path helm/go-mcp-platform/README.md

.PHONY: dagger-ci
dagger-ci: ## Run the full Dagger pipeline without publishing.
	dagger call ci --dry-run --version 0.0.0-dev export --path ./ci-out

.PHONY: dagger-test
dagger-test: ## Run the Dagger test gate and export coverage.
	dagger call test export --path ./coverage.out
