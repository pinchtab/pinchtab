#!/bin/bash
# Build and run PinchTab with React dashboard
set -euo pipefail

cd "$(dirname "$0")/.."

# Build dashboard
./scripts/build-dashboard.sh

# Build Go
echo "🔨 Building Go..."
go build -o pinchtab ./cmd/pinchtab

# Run
exec ./pinchtab "$@"
