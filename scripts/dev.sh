#!/bin/bash
# Build and run Pinchtab with React dashboard
set -e

cd "$(dirname "$0")/.."

# Build dashboard
./scripts/build-dashboard.sh

# Build Go
echo "🔨 Building Go..."
go build -o pinchtab ./cmd/pinchtab

# Run
echo "🦀 Starting Pinchtab..."
exec ./pinchtab "$@"
