#!/usr/bin/env bash
set -euo pipefail
cp "$(dirname "$0")/pre-commit" "$(git rev-parse --show-toplevel)/.git/hooks/pre-commit"
chmod +x "$(git rev-parse --show-toplevel)/.git/hooks/pre-commit"
echo "✅ Git hooks installed"
