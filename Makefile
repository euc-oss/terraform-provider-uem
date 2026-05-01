# Workspace ONE UEM Terraform Provider Makefile

# Provider details for local installation
PROVIDER_NAME=uem
PROVIDER_NAMESPACE=omnissa
PROVIDER_VERSION=0.1.0
OS=$(shell go env GOOS)
ARCH=$(shell go env GOARCH)

# OS-specific commands
ifeq ($(OS),windows)
    INSTALL_DIR_RAW=$(APPDATA)/terraform.d/plugins/local/$(PROVIDER_NAMESPACE)/$(PROVIDER_NAME)/$(PROVIDER_VERSION)/$(OS)_$(ARCH)
    # Convert forward slashes to backslashes for Windows
    INSTALL_DIR=$(subst /,\,$(INSTALL_DIR_RAW))
    BINARY_NAME=terraform-provider-$(PROVIDER_NAME)_v$(PROVIDER_VERSION).exe
else
    INSTALL_DIR=~/.terraform.d/plugins/local/$(PROVIDER_NAMESPACE)/$(PROVIDER_NAME)/$(PROVIDER_VERSION)/$(OS)_$(ARCH)
    BINARY_NAME=terraform-provider-$(PROVIDER_NAME)_v$(PROVIDER_VERSION)
endif

# Default target
default: help

# Help target - shows available commands
help:
	@echo "Workspace ONE Terraform Provider - Available Commands"
	@echo ""
	@echo "Development:"
	@echo "  make build             - Build the provider binary"
	@echo "  make install           - Build and install provider locally"
	@echo "  make fmt               - Format Go code"
	@echo "  make fmt-md            - Format Markdown files"
	@echo "  make lint              - Run golangci-lint"
	@echo "  make lint-md           - Run markdownlint"
	@echo "  make lint-md-report    - Generate detailed Markdown lint report"
	@echo "  make lint-all          - Run all linters (Go + Markdown)"
	@echo "  make test              - Run unit tests"
	@echo "  make testacc           - Run acceptance tests (requires TF_ACC=1)"
	@echo "  make generate          - Generate code from tools"
	@echo "  make clean             - Remove build artifacts"
	@echo ""
	@echo "Local Development:"
	@echo "  make dev-mode          - Use local SDK clone (reads UEM_SDK_PATH_LOCAL_DEV from .env)"
	@echo "  make release-mode      - Switch back to remote SDK (remove go.work)"
	@echo "  make clean-imports     - Remove terraform state/temp files from import examples"
	@echo ""
	@echo "Quick Checks (before commit/push):"
	@echo "  make quick-check       - Fast pre-commit checks (format, vet, test, build)"
	@echo "  make ci                - Full CI checks locally (mimics GitHub Actions)"
	@echo "  make pre-commit        - Alias for quick-check"
	@echo "  make pre-push          - Alias for ci"
	@echo ""
	@echo "Documentation:"
	@echo "  make docs              - Generate documentation"
	@echo "  make docs-check        - Check documentation for issues"
	@echo ""
	@echo "Utilities:"
	@echo "  make list-profiles     - List profiles from Workspace ONE"
	@echo "  make version           - Show provider version"
	@echo "  make help              - Show this help message"

# Build the provider
build:
	@echo "Building provider..."
	go build -v -o $(BINARY_NAME) .

# Install provider locally
install: build
	@echo "Installing provider to $(INSTALL_DIR)..."
ifeq ($(OS),windows)
	@powershell -Command "New-Item -ItemType Directory -Force -Path '$(INSTALL_DIR)' | Out-Null"
	@powershell -Command "Copy-Item -Path '$(BINARY_NAME)' -Destination '$(INSTALL_DIR)\$(BINARY_NAME)' -Force"
else
	@mkdir -p $(INSTALL_DIR)
	@cp $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@# Also write the no-version-suffix name that Terraform executes at runtime.
	@# Without this, Terraform loads a stale binary from a previous install.
	@cp $(BINARY_NAME) $(INSTALL_DIR)/terraform-provider-$(PROVIDER_NAME)
	@# Clear macOS quarantine/provenance attributes that cause Gatekeeper to block the plugin
	@xattr -cr $(INSTALL_DIR)/ 2>/dev/null || true
endif
	@echo "Provider installed successfully!"
	@echo "Location: $(INSTALL_DIR)/$(BINARY_NAME)"

# Format Go code
fmt:
	@echo "Formatting Go code..."
	gofmt -s -w -e .

# Format Markdown files
fmt-md:
	@echo "Formatting Markdown files..."
	@if ! command -v prettier >/dev/null 2>&1; then \
		echo "❌ prettier not found. Install with: npm install -g prettier"; \
		echo "⚠️  Skipping Markdown formatting"; \
	else \
		prettier --write "**/*.md" --prose-wrap always; \
		echo "✅ Markdown files formatted"; \
	fi

# Run Go linter
GOLANGCI_LINT_VERSION ?= v1.62.2

lint:
	@echo "Running golangci-lint..."
	@if ! command -v golangci-lint >/dev/null 2>&1 && ! [ -f ~/go/bin/golangci-lint ]; then \
		echo "❌ golangci-lint not found. Installing $(GOLANGCI_LINT_VERSION)..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
		echo "✅ golangci-lint installed to ~/go/bin/golangci-lint"; \
		echo "⚠️  Make sure ~/go/bin is in your PATH"; \
	fi
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=10m; \
	else \
		~/go/bin/golangci-lint run --timeout=10m; \
	fi

# Run Markdown linter
MARKDOWNLINT_VERSION ?= 0.43.0

lint-md:
	@echo "Running markdownlint..."
	@if ! command -v markdownlint >/dev/null 2>&1; then \
		echo "❌ markdownlint not found. Installing..."; \
		echo "⚠️  Installing markdownlint-cli@$(MARKDOWNLINT_VERSION) globally (requires npm)..."; \
		npm install -g markdownlint-cli@$(MARKDOWNLINT_VERSION) || (echo "❌ Failed to install markdownlint. Install manually: npm install -g markdownlint-cli@$(MARKDOWNLINT_VERSION)" && exit 1); \
	fi
	@markdownlint . --config .markdownlint.json --ignore-path .markdownlintignore || (echo "❌ Markdown linting failed. Run 'make fmt-md' to auto-fix some issues." && exit 1)

# Run Markdown linter with auto-fix
lint-md-fix:
	@echo "Running markdownlint with auto-fix..."
	@if ! command -v markdownlint >/dev/null 2>&1; then \
		echo "❌ markdownlint not found. Installing..."; \
		npm install -g markdownlint-cli@$(MARKDOWNLINT_VERSION) || (echo "❌ Failed to install markdownlint" && exit 1); \
	fi
	@markdownlint . --config .markdownlint.json --ignore-path .markdownlintignore --fix
	@echo "✅ Markdown files fixed"

# Generate detailed Markdown lint report
lint-md-report:
	@echo "Generating Markdown lint report..."
	@./scripts/markdown-lint-report.sh

# Run all linters
lint-all: lint lint-md

# Run unit tests
test:
	@echo "Running unit tests..."
	go test -v -cover -timeout=120s -parallel=10 ./...

# Run acceptance tests
testacc:
	@echo "Running acceptance tests..."
	TF_ACC=1 go test -v -cover -timeout 120m ./...

# Generate code
generate:
	@echo "Generating code..."
	cd tools; go generate ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
ifeq ($(OS),windows)
	@powershell -Command "Remove-Item -Force -ErrorAction SilentlyContinue $(BINARY_NAME)"
	@powershell -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue dist"
else
	@rm -f $(BINARY_NAME)
	@rm -rf dist/
endif
	@echo "Clean complete!"

# Quick check - Fast pre-commit checks
quick-check:
	@echo "Running quick pre-commit checks..."
	@./scripts/quick-check.sh

# CI - Full CI checks locally
ci:
	@echo "Running full CI checks locally..."
	@./scripts/run-ci-locally.sh

# Aliases for convenience
pre-commit: quick-check
pre-push: ci

# Generate documentation
docs:
	@echo "Generating documentation..."
	@echo "Documentation is in docs/ directory"

# Check documentation
docs-check:
	@echo "Checking documentation..."
	@markdownlint docs/ README.md || echo "markdownlint not installed, skipping..."

# List profiles from Workspace ONE
list-profiles:
	@echo "Listing profiles from Workspace ONE..."
	@cd scripts && go run list-profiles.go

list-application:
	@echo "Listing applications from Workspace ONE..."
	@cd scripts && go run list-application.go

# Show version
version:
	@echo "Provider: $(PROVIDER_NAME)"
	@echo "Version: $(PROVIDER_VERSION)"
	@echo "Namespace: $(PROVIDER_NAMESPACE)"
	@echo "OS/Arch: $(OS)/$(ARCH)"

# Verify dependencies
deps:
	@echo "Verifying dependencies..."
	go mod verify
	go mod tidy
	@cd scripts && go mod tidy

# Enable local SDK development mode via go.work (gitignored)
# Reads UEM_SDK_PATH_LOCAL_DEV from .env (default: ../terraform-sdk-uem)
dev-mode:
	@SDK_PATH=$$(grep '^UEM_SDK_PATH_LOCAL_DEV=' .env 2>/dev/null | cut -d= -f2 | tr -d '"' | tr -d "'"); \
	SDK_PATH=$${SDK_PATH:-../terraform-sdk-uem}; \
	if [ ! -d "$$SDK_PATH" ]; then \
		echo "Error: SDK path '$$SDK_PATH' does not exist"; \
		echo "Set UEM_SDK_PATH_LOCAL_DEV in .env or clone the SDK as a sibling directory"; \
		exit 1; \
	fi; \
	if [ ! -f "$$SDK_PATH/go.mod" ]; then \
		echo "Error: No go.mod found in '$$SDK_PATH' — is this a valid Go module?"; \
		exit 1; \
	fi; \
	if [ -f go.work ]; then \
		go work use . "$$SDK_PATH"; \
	else \
		go work init . "$$SDK_PATH"; \
	fi; \
	go env -w GOPRIVATE="github.com/euc-oss/*"; \
	echo "Local SDK enabled via go.work ($$SDK_PATH)"

# Switch to release mode (remove go.work)
release-mode:
ifeq ($(OS),windows)
	@powershell -Command "Remove-Item -Force -ErrorAction SilentlyContinue go.work, go.work.sum"
else
	@rm -f go.work go.work.sum
endif
	@echo "Local SDK disabled (using remote)"

# Remove terraform state/temp files from import examples
clean-imports:
	@echo "Cleaning import example artifacts..."
ifeq ($(OS),windows)
	@powershell -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue examples\resources\uem_profile\import-existing\.terraform"
	@powershell -Command "Remove-Item -Force -ErrorAction SilentlyContinue examples\resources\uem_profile\import-existing\.terraform.lock.hcl"
	@powershell -Command "Remove-Item -Force -ErrorAction SilentlyContinue examples\resources\uem_profile\import-existing\terraform.tfstate*"
	@powershell -Command "Remove-Item -Force -ErrorAction SilentlyContinue examples\resources\uem_profile\import-existing\import-temp-*.tf"
else
	@rm -rf examples/resources/uem_profile/import-existing/.terraform
	@rm -f  examples/resources/uem_profile/import-existing/.terraform.lock.hcl
	@rm -f  examples/resources/uem_profile/import-existing/terraform.tfstate*
	@rm -f  examples/resources/uem_profile/import-existing/import-temp-*.tf
endif
	@echo "Import artifacts cleaned!"

.PHONY: default help build install fmt fmt-md lint lint-md lint-md-fix \
        lint-md-report lint-all test testacc generate clean quick-check ci \
        pre-commit pre-push docs docs-check list-profiles version deps \
        dev-mode release-mode clean-imports

