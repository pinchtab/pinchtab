#!/bin/bash
# Build React dashboard and copy to internal/dashboard/dashboard/
set -e

cd "$(dirname "$0")/.."

# Generate TypeScript types from Go structs (ensures types are in sync)
TYGO="${GOPATH:-$HOME/go}/bin/tygo"
if [ -x "$TYGO" ]; then
  echo "🔄 Generating TypeScript types..."
  "$TYGO" generate
elif command -v tygo &> /dev/null; then
  echo "🔄 Generating TypeScript types..."
  tygo generate
else
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "⚠️  WARNING: tygo not found — skipping TypeScript type generation"
  echo "   Types in the dashboard might fall out of sync with Go structs."
  echo "   Install it with: go install github.com/gzuidhof/tygo@latest"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
fi

echo "📦 Building React dashboard..."
cd dashboard

# Install deps if needed
if [ ! -d "node_modules" ]; then
  echo "📥 Installing dependencies..."
  bun install --frozen-lockfile
fi

bun run build

echo "📋 Copying build to internal/dashboard/dashboard/..."
cd ..

# Clear old dashboard (keep directory)
rm -rf internal/dashboard/dashboard/*

# Copy React build
cp -r dashboard/dist/* internal/dashboard/dashboard/

# Rename index.html to dashboard.html (Go expects this)
mv internal/dashboard/dashboard/index.html internal/dashboard/dashboard/dashboard.html

echo "✅ Dashboard built: internal/dashboard/dashboard/"
