#!/usr/bin/env bash
# Quick Check - Fast pre-commit checks
# This script runs fast checks before committing code

# shellcheck source=scripts/common.sh disable=SC1091
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

# Track overall status
FAILED=0

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Quick Pre-Commit Checks${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Function to track failures
check_status() {
    if [ "$1" -ne 0 ]; then
        FAILED=1
    fi
    print_status "$1" "$2"
}

# 1. Go Format Check
echo -e "${YELLOW}[1/6]${NC} Checking Go formatting..."
if gofmt -l . | grep -q .; then
    echo -e "${RED}The following files need formatting:${NC}"
    gofmt -l .
    echo -e "${YELLOW}Run 'make fmt' to fix${NC}"
    check_status 1 "Go format check"
else
    check_status 0 "Go format check"
fi

# 2. Go Vet
echo ""
echo -e "${YELLOW}[2/6]${NC} Running go vet..."
if go vet ./...; then
    check_status 0 "Go vet"
else
    check_status 1 "Go vet"
fi

# 3. Go Mod Tidy Check
echo ""
echo -e "${YELLOW}[3/6]${NC} Checking go.mod tidiness..."
cp go.mod go.mod.backup
cp go.sum go.sum.backup
go mod tidy
if diff go.mod go.mod.backup > /dev/null && diff go.sum go.sum.backup > /dev/null; then
    check_status 0 "Go mod tidy"
    rm go.mod.backup go.sum.backup
else
    echo -e "${RED}go.mod or go.sum needs tidying${NC}"
    echo -e "${YELLOW}Run 'go mod tidy' to fix${NC}"
    mv go.mod.backup go.mod
    mv go.sum.backup go.sum
    check_status 1 "Go mod tidy"
fi

# 4. Unit Tests
echo ""
echo -e "${YELLOW}[4/6]${NC} Running unit tests..."
if go test -short -timeout=60s ./...; then
    check_status 0 "Unit tests"
else
    check_status 1 "Unit tests"
fi

# 5. Build Check
echo ""
echo -e "${YELLOW}[5/6]${NC} Building provider..."
if go build -v -o terraform-provider-uem .; then
    check_status 0 "Build"
    rm -f terraform-provider-uem
else
    check_status 1 "Build"
fi

# 6. Terraform Format Check (examples)
echo ""
echo -e "${YELLOW}[6/6]${NC} Checking Terraform formatting..."
if command -v terraform &> /dev/null; then
    if terraform fmt -check -recursive "$REPO_ROOT/examples/" > /dev/null 2>&1; then
        check_status 0 "Terraform format"
    else
        echo -e "${RED}Terraform files need formatting${NC}"
        echo -e "${YELLOW}Run 'terraform fmt -recursive examples/' to fix${NC}"
        check_status 1 "Terraform format"
    fi
else
    echo -e "${YELLOW}Terraform not installed, skipping format check${NC}"
    check_status 0 "Terraform format (skipped)"
fi

# Summary
echo ""
echo -e "${BLUE}========================================${NC}"
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All quick checks passed!${NC}"
    echo -e "${GREEN}Ready to commit.${NC}"
    echo ""
    echo -e "${BLUE}Next steps:${NC}"
    echo -e "  git add ."
    echo -e "  git commit -m \"Your commit message\""
    echo ""
    echo -e "${YELLOW}Before pushing, run:${NC} make pre-push"
    exit 0
else
    echo -e "${RED}✗ Some checks failed!${NC}"
    echo -e "${RED}Please fix the issues above before committing.${NC}"
    echo ""
    echo -e "${BLUE}Quick fixes:${NC}"
    echo -e "  make fmt           - Fix Go formatting"
    echo -e "  go mod tidy        - Fix go.mod"
    echo -e "  terraform fmt -recursive examples/ - Fix Terraform formatting"
    exit 1
fi
