#!/usr/bin/env bash
set -euo pipefail

ANTHROPIC_KEY=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --anthropic-key)
      ANTHROPIC_KEY=${2:-}
      shift 2
      ;;
    --anthropic-key=*)
      ANTHROPIC_KEY=${1#--anthropic-key=}
      shift
      ;;
    *)
      echo "unknown argument: $1" >&2
      echo "usage: run.sh [--anthropic-key <key>]" >&2
      exit 2
      ;;
  esac
done
if [[ -z "$ANTHROPIC_KEY" && -n "${ANTHROPIC_API_KEY:-}" ]]; then
  ANTHROPIC_KEY=$ANTHROPIC_API_KEY
fi

ROOT_DIR=$(cd "$(dirname "$0")/../../.." && pwd)
SMOKE_DIR="$ROOT_DIR/tests/manual/openclaw-plugin-smoke"
STATE_SOURCE=${OPENCLAW_STATE_SOURCE:-$HOME/.openclaw}
PROJECT_NAME=${PROJECT_NAME:-pinchtab-openclaw-mock-$(date +%s)}
TEMP_STATE=$(mktemp -d /tmp/pinchtab-openclaw-state.XXXXXX)
TEMP_ARTIFACTS=$(mktemp -d /tmp/pinchtab-openclaw-artifacts.XXXXXX)
FINAL_ARTIFACTS_DIR=${FINAL_ARTIFACTS_DIR:-$SMOKE_DIR/artifacts/$(date +%Y%m%d-%H%M%S)}

cleanup() {
  docker compose -p "$PROJECT_NAME" -f "$SMOKE_DIR/docker-compose.yml" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ -z "${OPENCLAW_VERSION:-}" ]]; then
  if command -v openclaw >/dev/null 2>&1; then
    OPENCLAW_VERSION=$(openclaw --version | awk '{print $2}')
  elif [[ -n "$ANTHROPIC_KEY" ]]; then
    OPENCLAW_VERSION=2026.4.27
  else
    echo "openclaw CLI not found on PATH" >&2
    echo "this smoke test reuses the host OpenClaw install for version detection and auth state." >&2
    echo "install openclaw locally, pin a version with OPENCLAW_VERSION=<version>," >&2
    echo "or pass --anthropic-key <key> to bootstrap a fresh in-container state." >&2
    exit 1
  fi
fi

if [[ -n "$ANTHROPIC_KEY" ]]; then
  echo "using --anthropic-key: bootstrapping fresh state in container (host openclaw state ignored)"
  for agent in main alpha beta; do
    mkdir -p "$TEMP_STATE/agents/$agent/agent"
  done
else
  require_file() {
    local path=$1
    if [[ ! -f "$path" ]]; then
      echo "missing required file: $path" >&2
      echo "this smoke test reuses the host OpenClaw state at $STATE_SOURCE." >&2
      echo "run 'openclaw login' (or set OPENCLAW_STATE_SOURCE) before retrying," >&2
      echo "or pass --anthropic-key <key> to bootstrap a fresh in-container state." >&2
      exit 1
    fi
  }

  require_file "$STATE_SOURCE/openclaw.json"
  require_file "$STATE_SOURCE/agents/main/agent/auth-profiles.json"

  for agent in main alpha beta; do
    mkdir -p "$TEMP_STATE/agents/$agent/agent"
  done
  cp "$STATE_SOURCE/openclaw.json" "$TEMP_STATE/openclaw.json"
  for agent in main alpha beta; do
    cp "$STATE_SOURCE/agents/main/agent/auth-profiles.json" "$TEMP_STATE/agents/$agent/agent/auth-profiles.json"
  done
  for dir in credentials identity gateway service-env; do
    if [[ -d "$STATE_SOURCE/$dir" ]]; then
      cp -R "$STATE_SOURCE/$dir" "$TEMP_STATE/$dir"
    fi
  done
fi

mkdir -p "$FINAL_ARTIFACTS_DIR"
export OPENCLAW_STATE_DIR="$TEMP_STATE"
export ARTIFACTS_DIR="$TEMP_ARTIFACTS"
export OPENCLAW_VERSION
export ANTHROPIC_API_KEY="$ANTHROPIC_KEY"

echo "building docker images..."
if ! docker compose -p "$PROJECT_NAME" -f "$SMOKE_DIR/docker-compose.yml" build --quiet >"$TEMP_ARTIFACTS/docker-build.log" 2>&1; then
  cat "$TEMP_ARTIFACTS/docker-build.log" >&2
  cp -R "$TEMP_ARTIFACTS/." "$FINAL_ARTIFACTS_DIR/"
  echo "build failed — artifacts: $FINAL_ARTIFACTS_DIR" >&2
  exit 1
fi

echo "running smoke..."
if ! docker compose -p "$PROJECT_NAME" -f "$SMOKE_DIR/docker-compose.yml" up --abort-on-container-exit --exit-code-from openclaw 2>&1 | tee "$TEMP_ARTIFACTS/docker-compose.log"; then
  cp -R "$TEMP_ARTIFACTS/." "$FINAL_ARTIFACTS_DIR/"
  echo
  echo "failed — artifacts copied to $FINAL_ARTIFACTS_DIR" >&2
  exit 1
fi

cp -R "$TEMP_ARTIFACTS/." "$FINAL_ARTIFACTS_DIR/"
echo
echo "artifacts: $FINAL_ARTIFACTS_DIR"
