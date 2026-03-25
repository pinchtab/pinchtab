#!/bin/bash
# dev-dashboard.sh — full-stack dashboard development with frontend hot reload
# and backend auto-rebuild/restart.
#
# This starts:
#   1. pinchtab backend (Go) on port 9867, rebuilt on file changes
#   2. Vite dev server (React) on port 5173 with proxy to backend
#
# Access the dashboard at:
#   - http://localhost:5173/dashboard/ (use this during development)
#
# Usage: ./scripts/dev-dashboard.sh [pinchtab server args...]

set -euo pipefail

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
MUTED=$'\033[38;2;90;100;128m'
SUCCESS=$'\033[38;2;0;229;204m'
ERROR=$'\033[38;2;255;107;107m'
NC=$'\033[0m'

DEV_PORT=${PINCHTAB_DEV_PORT:-9867}
SERVER_ARGS=("$@")
WATCH_INTERVAL_SECONDS="${PINCHTAB_DEV_WATCH_INTERVAL:-1}"
DEV_TOKEN=""
DEV_CONFIG=""
BACKEND_PID=""
VITE_PID=""
LAST_BACKEND_SIGNATURE=""
GO_BUILD_CACHE_DIR="${PWD}/.gocache/build"
GO_MOD_CACHE_DIR="${PWD}/.gocache/mod"

cleanup() {
  trap - SIGINT SIGTERM EXIT
  echo ""
  echo "  ${MUTED}Shutting down...${NC}"
  if [ -n "${BACKEND_PID:-}" ]; then
    kill "${BACKEND_PID}" 2>/dev/null || true
    wait "${BACKEND_PID}" 2>/dev/null || true
  fi
  if [ -n "${VITE_PID:-}" ]; then
    kill "${VITE_PID}" 2>/dev/null || true
    wait "${VITE_PID}" 2>/dev/null || true
  fi
  if [ -n "${DEV_CONFIG:-}" ] && [ -f "${DEV_CONFIG}" ]; then
    rm -f "${DEV_CONFIG}" 2>/dev/null || true
  fi
  exit 0
}

trap cleanup SIGINT SIGTERM EXIT

watch_signature() {
  local files=()
  while IFS= read -r file; do
    files+=("$file")
  done < <(
    find cmd internal \
      -type f \
      \( \
        -name '*.go' -o \
        -name '*.js' -o \
        -name '*.json' \
      \) \
      | sort
  )

  files+=("go.mod" "go.sum")

  if [ "${#files[@]}" -eq 0 ]; then
    echo ""
    return
  fi

  shasum "${files[@]}" 2>/dev/null | shasum | awk '{print $1}'
}

generate_dev_token() {
  local token=""
  token="$(od -An -N24 -tx1 /dev/urandom 2>/dev/null | tr -d ' \n')"
  if [ -z "${token}" ]; then
    echo "  ${ERROR}✗${NC} Failed to generate a dev server token."
    return 1
  fi
  DEV_TOKEN="${token}"
  return 0
}

build_backend() {
  local target="./pinchtab-dev"
  echo "  ${MUTED}Building Go backend...${NC}"
  mkdir -p "${GO_BUILD_CACHE_DIR}" "${GO_MOD_CACHE_DIR}"
  if GOCACHE="${GO_BUILD_CACHE_DIR}" GOMODCACHE="${GO_MOD_CACHE_DIR}" go build -o "${target}" ./cmd/pinchtab; then
    echo "  ${SUCCESS}✓${NC} Backend build succeeded"
    return 0
  fi

  echo "  ${ERROR}✗${NC} Backend build failed. Keeping the last good server running."
  return 1
}

resolve_dev_token() {
  DEV_TOKEN="dev"
  # generate_dev_token
}

wait_for_backend() {
  local tries=0
  while [ "${tries}" -lt 30 ]; do
    if curl -fsS -H "Authorization: Bearer ${DEV_TOKEN}" "http://localhost:${DEV_PORT}/health" >/dev/null 2>&1; then
      return 0
    fi
    tries=$((tries + 1))
    sleep 0.5
  done
  return 1
}

start_backend() {
  if ! build_backend; then
    return 1
  fi

  if [ -n "${BACKEND_PID:-}" ]; then
    kill "${BACKEND_PID}" 2>/dev/null || true
    wait "${BACKEND_PID}" 2>/dev/null || true
    BACKEND_PID=""
  fi

  echo "  ${MUTED}Starting pinchtab backend on :${DEV_PORT}...${NC}"
  PINCHTAB_CONFIG="${DEV_CONFIG}" PINCHTAB_TOKEN="${DEV_TOKEN}" ./pinchtab-dev server "${SERVER_ARGS[@]}" &
  BACKEND_PID=$!

  if wait_for_backend; then
    echo "  ${SUCCESS}✓${NC} Backend ready"
    return 0
  fi

  echo "  ${ERROR}✗${NC} Backend failed to start"
  kill "${BACKEND_PID}" 2>/dev/null || true
  wait "${BACKEND_PID}" 2>/dev/null || true
  BACKEND_PID=""
  return 1
}

start_vite() {
  echo "  ${MUTED}Starting Vite dev server on :5173...${NC}"
  cd dashboard

  if command -v bun >/dev/null 2>&1; then
    PINCHTAB_DEV_PORT="${DEV_PORT}" PINCHTAB_TOKEN="${DEV_TOKEN}" bun run dev &
  else
    PINCHTAB_DEV_PORT="${DEV_PORT}" PINCHTAB_TOKEN="${DEV_TOKEN}" npm run dev &
  fi
  VITE_PID=$!

  cd ..
}

monitor_backend_changes() {
  LAST_BACKEND_SIGNATURE="$(watch_signature)"

  while true; do
    sleep "${WATCH_INTERVAL_SECONDS}"

    if [ -n "${BACKEND_PID:-}" ] && ! kill -0 "${BACKEND_PID}" 2>/dev/null; then
      echo ""
      echo "  ${ERROR}Backend process exited. Restarting...${NC}"
      start_backend || true
      LAST_BACKEND_SIGNATURE="$(watch_signature)"
      continue
    fi

    local current_signature
    current_signature="$(watch_signature)"
    if [ "${current_signature}" = "${LAST_BACKEND_SIGNATURE}" ]; then
      continue
    fi

    LAST_BACKEND_SIGNATURE="${current_signature}"
    echo ""
    echo "  ${ACCENT}${BOLD}↻ Change detected${NC}"
    start_backend || true
  done
}

echo ""
echo "  ${ACCENT}${BOLD}🔥 Dashboard Full-Stack Dev Mode${NC}"
echo ""

if ! resolve_dev_token; then
  exit 1
fi

DEV_CONFIG="$(mktemp -t pinchtab-dev-config.XXXXXX)"
cat > "${DEV_CONFIG}" <<EOF
{
  "configVersion": "0.8.0",
  "server": {
    "bind": "127.0.0.1",
    "port": "${DEV_PORT}",
    "token": "${DEV_TOKEN}"
  },
  "security": {
    "allowEvaluate": true,
    "allowMacro": true,
    "allowScreencast": true,
    "allowDownload": true,
    "allowUpload": true,
    "idpi": {
      "enabled": false,
      "strictMode": false,
      "scanContent": false,
      "wrapContent": false
    }
  }
}
EOF

if ! start_backend; then
  exit 1
fi

echo ""
start_vite

echo ""
echo "  ${SUCCESS}${BOLD}✓ Ready!${NC}"
echo ""
echo "  ${BOLD}Use:${NC}        ${ACCENT}http://localhost:5173/dashboard/${NC}"
echo "  ${BOLD}Backend:${NC}    http://localhost:${DEV_PORT}"
echo ""
echo "  ${MUTED}Frontend changes hot-reload through Vite.${NC}"
echo "  ${MUTED}Backend changes rebuild and restart the server automatically.${NC}"
echo "  ${MUTED}The Vite proxy uses the isolated dev token automatically.${NC}"
echo "  ${MUTED}Press Ctrl+C to stop.${NC}"
echo ""

monitor_backend_changes
