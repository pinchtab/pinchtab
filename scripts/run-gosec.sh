#!/bin/bash
# Run gosec security scan locally with the same config as CI
set -e

# Ensure ~/go/bin is in PATH
export PATH="$HOME/go/bin:$PATH"

# Check if gosec is installed
if ! command -v gosec &> /dev/null; then
    echo "Installing gosec..."
    go install github.com/securego/gosec/v2/cmd/gosec@latest
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Running gosec security scan..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "⏱️  This may take 3-5 minutes on local machines"
echo "   (CI runs faster on optimized GitHub runners)"
echo ""

START_TIME=$(date +%s)

# Run gosec with the same config as CI
# Note: This scans all Go code, same as the CI does
gosec -exclude=G301,G302,G304,G306,G404,G107,G115,G703,G704,G705,G706 \
  -fmt=json \
  -out=gosec-results.json \
  ./... 2>&1 | grep -E "Checking (file|package)" | tail -10 || true

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Security Scan Results (${DURATION}s)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Check if results file exists
if [ ! -f gosec-results.json ]; then
  echo "❌ No results file generated (scan may have failed)"
  exit 1
fi

cat gosec-results.json | jq -r '.Stats'
echo ""

# Check for critical findings (same as CI)
ISSUES=$(cat gosec-results.json | jq '[.Issues[] | select(.rule_id == "G112" or .rule_id == "G204")] | length')
echo "Critical issues (G112, G204): $ISSUES"

if [ "$ISSUES" -gt 0 ]; then
  echo ""
  echo "❌ CRITICAL ISSUES FOUND (will fail CI):"
  cat gosec-results.json | jq -r '.Issues[] | select(.rule_id == "G112" or .rule_id == "G204")'
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  exit 1
else
  echo "✅ No critical issues (CI will pass)"
  
  TOTAL=$(cat gosec-results.json | jq '.Stats.found')
  if [ "$TOTAL" -gt 0 ]; then
    echo ""
    echo "ℹ️  Other issues found (excluded from CI):"
    cat gosec-results.json | jq -r '.Issues[0:5] | .[] | "  \(.severity): \(.rule_id) at \(.file):\(.line)"'
    echo ""
    echo "Full report: gosec-results.json"
  fi
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
