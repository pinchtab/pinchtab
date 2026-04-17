#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <npm-install-target>" >&2
  exit 1
fi

INSTALL_TARGET="$1"
TMP_ROOT="$(mktemp -d)"
HOME_DIR="$TMP_ROOT/home"
PREFIX_DIR="$TMP_ROOT/prefix"
BIN_DIR="$PREFIX_DIR/bin"
SERVER_LOG="$TMP_ROOT/server.log"
PORT="${PINCHTAB_SMOKE_PORT:-19867}"
SERVER_PID=""

cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

mkdir -p "$HOME_DIR" "$PREFIX_DIR"

export HOME="$HOME_DIR"
export PATH="$BIN_DIR:$PATH"

echo "Installing npm package from $INSTALL_TARGET"
npm install -g --prefix "$PREFIX_DIR" "$INSTALL_TARGET"

echo "Initializing CLI config"
pinchtab config init
pinchtab config set server.port "$PORT"

CONFIG_PATH="$(pinchtab config path)"
TOKEN="$(node -pe "require(process.argv[1]).server.token" "$CONFIG_PATH")"

if [ -z "$TOKEN" ]; then
  echo "smoke failed: expected non-empty server token in $CONFIG_PATH" >&2
  exit 1
fi

echo "Starting PinchTab server on port $PORT"
pinchtab server >"$SERVER_LOG" 2>&1 &
SERVER_PID="$!"

for _ in $(seq 1 60); do
  if curl -fsS -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:$PORT/health" >/dev/null; then
    echo "CLI auth smoke passed"
    exit 0
  fi
  sleep 1
done

echo "smoke failed: server never accepted the configured token" >&2
echo "server log:" >&2
cat "$SERVER_LOG" >&2
exit 1
