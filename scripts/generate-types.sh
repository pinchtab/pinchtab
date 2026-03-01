#!/bin/bash
# Generate TypeScript types from Go structs using tygo
set -e

cd "$(dirname "$0")/.."

# Ensure tygo is installed
if ! command -v tygo &> /dev/null; then
    echo "Installing tygo..."
    go install github.com/gzuidhof/tygo@latest
fi

# Generate types
echo "Generating TypeScript types..."
tygo generate

echo "✓ Types generated: dashboard/src/generated/types.ts"
