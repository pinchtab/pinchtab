#!/bin/bash
# Build React dashboard and copy to internal/dashboard/dashboard/
set -e

cd "$(dirname "$0")/.."

echo "Building React dashboard..."
cd dashboard
npm run build

echo "Copying build to internal/dashboard/dashboard/..."
cd ..

# Backup assets we want to keep
cp internal/dashboard/dashboard/pinchtab-headed-192.png /tmp/pinchtab-headed-192.png

# Clear old dashboard (keep directory)
rm -rf internal/dashboard/dashboard/*

# Copy React build
cp -r dashboard/dist/* internal/dashboard/dashboard/

# Restore assets
cp /tmp/pinchtab-headed-192.png internal/dashboard/dashboard/

# Rename index.html to dashboard.html (Go expects this)
mv internal/dashboard/dashboard/index.html internal/dashboard/dashboard/dashboard.html

echo "✓ Dashboard built and copied to internal/dashboard/dashboard/"
