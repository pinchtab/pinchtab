#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "📦 Plugin checks"
echo ""

# Verify JSON files
echo "  Validating JSON schemas..."
node -e "JSON.parse(require('fs').readFileSync('plugin/package.json', 'utf8'))"
node -e "JSON.parse(require('fs').readFileSync('plugin/openclaw.plugin.json', 'utf8'))"
echo "  ✓ JSON valid"

# Verify package contents
echo ""
echo "  Verifying package contents..."
cd plugin
npm pack --dry-run 2>&1 | grep -E "^npm notice [0-9]" | awk '{print "    " $4 " (" $3 ")"}'
echo "  ✓ Package verified"

echo ""
echo "✅ Plugin checks passed"
