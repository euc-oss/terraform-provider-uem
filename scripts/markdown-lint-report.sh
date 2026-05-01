#!/usr/bin/env bash
# Generate Markdown linting report

# shellcheck source=scripts/common.sh disable=SC1091
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

echo -e "${BLUE}=== Markdown Linting Report ===${NC}"
echo ""

# Run markdownlint and capture output
LINT_OUTPUT=$(markdownlint . --config .markdownlint.json --ignore-path .markdownlintignore 2>&1 || true)

if [ -z "$LINT_OUTPUT" ]; then
    echo -e "${GREEN}No Markdown linting issues found!${NC}"
    exit 0
fi

# Count total issues
TOTAL_ISSUES=$(echo "$LINT_OUTPUT" | wc -l | tr -d ' ')
echo -e "${YELLOW}Total Issues: $TOTAL_ISSUES${NC}"
echo ""

# Count issues by type
echo -e "${BLUE}Issues by Type:${NC}"
echo "$LINT_OUTPUT" | grep -oE 'MD[0-9]+/[a-z-]+' | sort | uniq -c | sort -rn | while read -r count rule; do
    echo "  $count x $rule"
done
echo ""

# Count files with issues
FILES_WITH_ISSUES=$(echo "$LINT_OUTPUT" | cut -d':' -f1 | sort -u | wc -l | tr -d ' ')
echo -e "${BLUE}Files with Issues: $FILES_WITH_ISSUES${NC}"
echo ""

# List files with issue counts
echo -e "${BLUE}Files Requiring Updates:${NC}"
echo "$LINT_OUTPUT" | cut -d':' -f1 | sort | uniq -c | sort -rn | while read -r count file; do
    echo "  $count issues - $file"
done
echo ""

# Show detailed breakdown by file
echo -e "${BLUE}Detailed Breakdown:${NC}"
echo ""

echo "$LINT_OUTPUT" | cut -d':' -f1 | sort -u | while read -r file; do
    echo -e "${YELLOW}$file${NC}"
    echo "$LINT_OUTPUT" | grep "^$file:" | sed 's/^/  /'
    echo ""
done

# Save to file
OUTPUT_DIR="$REPO_ROOT/output"
mkdir -p "$OUTPUT_DIR"

echo "$LINT_OUTPUT" > "$OUTPUT_DIR/markdown-lint-full.txt"
echo -e "${GREEN}Full report saved to: $OUTPUT_DIR/markdown-lint-full.txt${NC}"

# Generate summary by rule type
echo ""
echo -e "${BLUE}Quick Fix Guide:${NC}"
echo ""
echo "MD013 (Line Length):"
echo "  - Break long lines"
echo "  - Wrap URLs in angle brackets: <https://example.com>"
echo ""
echo "MD040 (Code Language):"
echo "  - Add language to code blocks: \`\`\`bash or \`\`\`terraform"
echo ""
echo "MD036 (Emphasis as Heading):"
echo "  - Convert **bold** to proper headings: ### Heading"
echo ""
echo "MD024 (Duplicate Headings):"
echo "  - Make headings unique or add context"
echo ""
echo -e "${BLUE}Commands:${NC}"
echo "  make lint-md          - Run linter"
echo "  make lint-md-fix      - Auto-fix issues"
echo "  make lint-all         - Run all linters"
