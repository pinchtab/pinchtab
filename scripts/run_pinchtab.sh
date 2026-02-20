#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
BIN_PATH="$BIN_DIR/pinchtab"

mkdir -p "$BIN_DIR"
cd "$ROOT_DIR"

go build -o "$BIN_PATH" ./cmd/pinchtab

if [ "$#" -eq 0 ]; then
  set -- dashboard
fi

exec "$BIN_PATH" "$@"
