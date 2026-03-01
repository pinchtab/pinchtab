#!/bin/bash
# Generate API reference documentation from Go handler definitions
# Usage: ./scripts/generate-api-docs.sh > docs/references/endpoints-generated.md

set -e

cd "$(dirname "$0")/.."

echo "# API Reference (Auto-Generated)"
echo ""
echo "This documentation is generated from Go code."
echo "Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""
echo "---"
echo ""

echo "## Endpoints Summary"
echo ""
echo "| Method | Path | Handler |"
echo "|--------|------|---------|"

# Extract routes from handlers.go
grep -E 'mux\.Handle(Func)?\(' internal/handlers/handlers.go | \
  sed -E 's/.*mux\.HandleFunc?\("([A-Z]+) ([^"]+)".*h\.([A-Za-z]+).*/| \1 | \2 | \3 |/' | \
  sort

echo ""
echo "---"
echo ""
echo "## Detailed Endpoints"
echo ""

# Extract each route and try to find documentation
while IFS= read -r line; do
  if [[ $line =~ mux\.HandleFunc\(\"([A-Z]+)[[:space:]]+([^\"]+)\".*h\.([A-Za-z0-9]+) ]]; then
    METHOD="${BASH_REMATCH[1]}"
    PATH="${BASH_REMATCH[2]}"
    HANDLER="${BASH_REMATCH[3]}"

    # Find the handler in the codebase
    HANDLER_FILE=$(grep -l "func.*($HANDLER" internal/handlers/*.go 2>/dev/null | head -1)

    if [ -n "$HANDLER_FILE" ]; then
      # Extract comments before the handler
      COMMENT=$(grep -B 5 "func.*($HANDLER" "$HANDLER_FILE" | grep "^[[:space:]]*\/\/" | head -1 | sed 's/^[[:space:]]*\/\/[[:space:]]*//')

      echo "### $METHOD $PATH"
      echo ""
      if [ -n "$COMMENT" ]; then
        echo "$COMMENT"
        echo ""
      fi
      echo "\`\`\`"
      echo "$METHOD $PATH"
      echo "\`\`\`"
      echo ""
    fi
  fi
done < <(grep 'mux\.HandleFunc' internal/handlers/handlers.go | head -20)

echo "---"
echo ""
echo "## Notes"
echo ""
echo "- This documentation is auto-generated from Go code"
echo "- For full details, see inline comments in \`internal/handlers/*.go\`"
echo "- Query parameters and request bodies vary by endpoint"
echo ""
