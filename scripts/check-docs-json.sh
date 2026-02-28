#!/bin/bash
# Validate docs.json - check that all referenced files exist

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

DOCS_JSON="docs/docs.json"
DOCS_DIR="docs"

# Check if docs.json exists
if [ ! -f "$DOCS_JSON" ]; then
    echo -e "${YELLOW}⚠️  $DOCS_JSON not found - skipping validation${NC}"
    exit 0
fi

# Extract all file paths from docs.json
# Looks for patterns like "path/to/file.md"
files=$(grep -oE '"[a-zA-Z0-9/_.-]+\.md"' "$DOCS_JSON" | tr -d '"' | sort -u)

if [ -z "$files" ]; then
    echo -e "${YELLOW}⚠️  No markdown files found in $DOCS_JSON${NC}"
    exit 0
fi

missing_files=()

# Check each file exists
for file in $files; do
    full_path="$DOCS_DIR/$file"
    
    if [ ! -f "$full_path" ]; then
        missing_files+=("$file")
    fi
done

# Report results
if [ ${#missing_files[@]} -eq 0 ]; then
    count=$(echo "$files" | wc -l)
    echo -e "${GREEN}✓ Documentation index valid${NC} ($count files)"
    exit 0
else
    echo -e "${RED}✗ Missing documentation files referenced in docs.json:${NC}"
    for file in "${missing_files[@]}"; do
        echo -e "${RED}  - $DOCS_DIR/$file${NC}"
    done
    echo ""
    echo -e "${YELLOW}Fix: Create missing files or update docs/docs.json${NC}"
    exit 1
fi
