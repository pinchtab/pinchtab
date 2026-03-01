#!/bin/bash

# Format all Go code with gofmt before committing
# Usage: ./scripts/format.sh

set -e

echo "🦀 Running gofmt on all Go files..."
gofmt -w .

echo "✓ All files formatted"
echo ""
echo "Next steps:"
echo "  1. Review changes: git status"
echo "  2. Stage changes: git add ."
echo "  3. Commit: git commit -m '...'"
echo ""
echo "Or use: git add . && git commit (pre-commit hook will verify format)"
