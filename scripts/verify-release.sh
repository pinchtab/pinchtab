#!/bin/bash
# verify-release.sh - Validate that a release has all expected binary permutations
#
# Usage:
#   ./scripts/verify-release.sh [TAG]  # Verify specific tag (default: latest)
#   ./scripts/verify-release.sh v0.7.6
#
# This script ensures:
#   1. All 6 binary permutations are present
#   2. checksums.txt exists
#   3. Checksums are valid (optional, requires local binaries)

set -e

REPO="${REPO:-pinchtab/pinchtab}"
TAG="${1:-}"  # Empty = latest

# Expected binary names (must match .goreleaser.yml)
EXPECTED_BINARIES=(
  "pinchtab-linux-amd64"
  "pinchtab-linux-arm64"
  "pinchtab-darwin-amd64"
  "pinchtab-darwin-arm64"
  "pinchtab-windows-amd64.exe"
  "pinchtab-windows-arm64.exe"
)

echo "=== Release Binary Verification ==="
echo ""

# Get release info
if [ -z "$TAG" ]; then
  echo "📦 Fetching latest release..."
  RELEASE=$(gh release view --repo "$REPO" --json assets,tagName)
else
  echo "📦 Fetching release: $TAG"
  RELEASE=$(gh release view "$TAG" --repo "$REPO" --json assets,tagName 2>/dev/null) || {
    echo "❌ Release not found: $TAG"
    exit 1
  }
fi

TAG_NAME=$(echo "$RELEASE" | jq -r '.tagName')
echo "   Tag: $TAG_NAME"
echo ""

# Extract asset names
ASSETS=$(echo "$RELEASE" | jq -r '.assets[].name')

# Check for each expected binary
FOUND=0
MISSING=()

echo "Checking binaries:"
for BINARY in "${EXPECTED_BINARIES[@]}"; do
  if echo "$ASSETS" | grep -q "^${BINARY}$"; then
    echo "  ✓ $BINARY"
    ((FOUND++))
  else
    echo "  ✗ $BINARY (MISSING)"
    MISSING+=("$BINARY")
  fi
done

EXPECTED_COUNT=${#EXPECTED_BINARIES[@]}

echo ""
echo "Summary:"
echo "  Expected: $EXPECTED_COUNT"
echo "  Found:    $FOUND"

# Check for checksums
if echo "$ASSETS" | grep -q "^checksums.txt$"; then
  echo "  ✓ checksums.txt present"
else
  echo "  ✗ checksums.txt missing"
fi

echo ""

# Status
if [ $FOUND -eq $EXPECTED_COUNT ] && echo "$ASSETS" | grep -q "^checksums.txt$"; then
  echo "✅ All binaries present ($FOUND/$EXPECTED_COUNT)"
  exit 0
else
  if [ ${#MISSING[@]} -gt 0 ]; then
    echo "❌ Missing binaries:"
    for BIN in "${MISSING[@]}"; do
      echo "   - $BIN"
    done
  fi
  exit 1
fi
