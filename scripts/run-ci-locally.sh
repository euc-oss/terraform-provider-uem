#!/usr/bin/env bash
# Local CI Runner - Run GitHub Actions checks locally before pushing
# This script mimics the GitHub Actions workflows for comprehensive testing

# shellcheck source=scripts/common.sh disable=SC1091
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

# Track overall status
FAILED=0

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Local CI Runner${NC}"
echo -e "${BLUE}Mimicking GitHub Actions Workflows${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Function to track failures
check_status() {
    if [ "$1" -ne 0 ]; then
        FAILED=1
    fi
    print_status "$1" "$2"
}

# Check if act is installed
if command -v act &> /dev/null; then
    USE_ACT=true
    echo -e "${GREEN}✓ act is installed - will use Docker-based GitHub Actions runner${NC}"
else
    USE_ACT=false
    echo -e "${YELLOW}⚠ act is not installed - will run checks manually${NC}"
    echo -e "${YELLOW}Install act for better GitHub Actions simulation: brew install act${NC}"
fi

echo ""

# ============================================
# LINT WORKFLOW
# ============================================
print_section "Lint Workflow (4 jobs)"

# Job 1: golangci-lint
echo ""
echo -e "${YELLOW}[1/4]${NC} Running golangci-lint..."
if command -v golangci-lint &> /dev/null; then
    if golangci-lint run; then
        check_status 0 "golangci-lint"
    else
        check_status 1 "golangci-lint"
    fi
else
    echo -e "${YELLOW}golangci-lint not installed, skipping...${NC}"
    echo -e "${YELLOW}Install: brew install golangci-lint${NC}"
    check_status 0 "golangci-lint (skipped)"
fi

# Job 2: terraform fmt
echo ""
echo -e "${YELLOW}[2/4]${NC} Checking Terraform formatting..."
if command -v terraform &> /dev/null; then
    if terraform fmt -check -recursive "$REPO_ROOT/examples/"; then
        check_status 0 "terraform fmt"
    else
        echo -e "${RED}Terraform files need formatting${NC}"
        check_status 1 "terraform fmt"
    fi
else
    echo -e "${YELLOW}Terraform not installed, skipping...${NC}"
    check_status 0 "terraform fmt (skipped)"
fi

# Job 3: markdown lint
echo ""
echo -e "${YELLOW}[3/4]${NC} Running markdown lint..."
if command -v markdownlint &> /dev/null; then
    if markdownlint '**/*.md' --ignore node_modules --ignore .terraform; then
        check_status 0 "markdown lint"
    else
        check_status 1 "markdown lint"
    fi
else
    echo -e "${YELLOW}markdownlint not installed, skipping...${NC}"
    echo -e "${YELLOW}Install: npm install -g markdownlint-cli${NC}"
    check_status 0 "markdown lint (skipped)"
fi

# Job 4: go mod tidy
echo ""
echo -e "${YELLOW}[4/4]${NC} Checking go mod tidy..."
cp go.mod go.mod.backup
cp go.sum go.sum.backup
go mod tidy
if diff go.mod go.mod.backup > /dev/null && diff go.sum go.sum.backup > /dev/null; then
    check_status 0 "go mod tidy"
    rm go.mod.backup go.sum.backup
else
    echo -e "${RED}go.mod or go.sum needs tidying${NC}"
    mv go.mod.backup go.mod
    mv go.sum.backup go.sum
    check_status 1 "go mod tidy"
fi

# ============================================
# TEST WORKFLOW
# ============================================
print_section "Test Workflow (3 jobs)"

# Job 1: Unit Tests
echo ""
echo -e "${YELLOW}[1/3]${NC} Running unit tests..."
if go test -v -cover -timeout=120s -parallel=10 ./...; then
    check_status 0 "unit tests"
else
    check_status 1 "unit tests"
fi

# Job 2: Acceptance Tests (optional - requires credentials)
echo ""
echo -e "${YELLOW}[2/3]${NC} Acceptance tests..."
if [ -n "${TF_ACC:-}" ] && [ "$TF_ACC" = "1" ]; then
    echo "Running acceptance tests (TF_ACC=1)..."
    if TF_ACC=1 go test -v -cover -timeout 120m ./...; then
        check_status 0 "acceptance tests"
    else
        check_status 1 "acceptance tests"
    fi
else
    echo -e "${YELLOW}Skipping acceptance tests (set TF_ACC=1 to run)${NC}"
    check_status 0 "acceptance tests (skipped)"
fi

# Job 3: Example Tests
echo ""
echo -e "${YELLOW}[3/3]${NC} Validating examples..."
if command -v terraform &> /dev/null; then
    EXAMPLE_FAILED=0
    # Check subdirectories that have main.tf files (actual examples)
    for example_dir in "$REPO_ROOT"/examples/resources/*/*/; do
        if [ -d "$example_dir" ] && [ -f "$example_dir/main.tf" ]; then
            parent_dir=$(basename "$(dirname "$example_dir")")
            current_dir=$(basename "$example_dir")
            echo "  Checking ${parent_dir}/${current_dir}..."
            (cd "$example_dir" && terraform init -backend=false > /dev/null 2>&1 && terraform validate > /dev/null 2>&1) || EXAMPLE_FAILED=1
        fi
    done
    if [ $EXAMPLE_FAILED -eq 0 ]; then
        check_status 0 "example validation"
    else
        check_status 1 "example validation"
    fi
else
    echo -e "${YELLOW}Terraform not installed, skipping example validation${NC}"
    check_status 0 "example validation (skipped)"
fi

# ============================================
# BUILD CHECK
# ============================================
print_section "Build Check"

echo ""
echo -e "${YELLOW}Building provider...${NC}"
if go build -v -o terraform-provider-uem .; then
    check_status 0 "build"
    rm -f terraform-provider-uem
else
    check_status 1 "build"
fi

# ============================================
# SUMMARY
# ============================================
print_section "Summary"

echo ""
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓✓✓ All CI checks passed! ✓✓✓${NC}"
    echo -e "${GREEN}Ready to push to GitHub.${NC}"
    echo ""
    echo -e "${BLUE}Next steps:${NC}"
    echo -e "  git push origin <branch>"
    echo ""
    if [ "$USE_ACT" = false ]; then
        echo -e "${YELLOW}Tip: Install 'act' for better GitHub Actions simulation:${NC}"
        echo -e "  brew install act"
    fi
    exit 0
else
    echo -e "${RED}✗✗✗ Some CI checks failed! ✗✗✗${NC}"
    echo -e "${RED}Please fix the issues above before pushing.${NC}"
    echo ""
    echo -e "${BLUE}Quick fixes:${NC}"
    echo -e "  make fmt                              - Fix Go formatting"
    echo -e "  go mod tidy                           - Fix go.mod"
    echo -e "  terraform fmt -recursive examples/    - Fix Terraform formatting"
    echo -e "  golangci-lint run --fix               - Auto-fix linting issues"
    exit 1
fi
