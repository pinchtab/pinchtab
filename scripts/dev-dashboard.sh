#!/bin/bash
# dev-dashboard.sh — Run pinchtab + Vite dev server for hot-reload dashboard development
#
# This starts:
#   1. pinchtab backend (Go) on port 9867
#   2. Vite dev server (React) on port 5173 with proxy to backend
#
# Access the dashboard at: http://localhost:5173/dashboard/
# Changes to dashboard/src/* will hot-reload instantly.
#
# Usage: ./scripts/dev-dashboard.sh [pinchtab args...]

set -e

cd "$(dirname "$0")/.."

BOLD=$'\033[1m'
ACCENT=$'\033[38;2;251;191;36m'
MUTED=$'\033[38;2;90;100;128m'
SUCCESS=$'\033[38;2;0;229;204m'
NC=$'\033[0m'

cleanup() {
  echo ""
  echo "  ${MUTED}Shutting down...${NC}"
  kill $BACKEND_PID 2>/dev/null || true
  kill $VITE_PID 2>/dev/null || true
  exit 0
}

trap cleanup SIGINT SIGTERM

echo ""
echo "  ${ACCENT}${BOLD}🔥 Dashboard Hot-Reload Dev Mode${NC}"
echo ""

# Build Go binary (without embedded dashboard - it won't be used)
echo "  ${MUTED}Building Go backend...${NC}"
go build -o pinchtab ./cmd/pinchtab

# Start backend
echo "  ${MUTED}Starting pinchtab backend on :9867...${NC}"
./pinchtab server "$@" &
BACKEND_PID=$!

# Wait for backend to be ready
echo "  ${MUTED}Waiting for backend...${NC}"
for i in {1..30}; do
  if curl -s http://localhost:9867/health >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

if ! curl -s http://localhost:9867/health >/dev/null 2>&1; then
  echo "  ${BOLD}Backend failed to start${NC}"
  kill $BACKEND_PID 2>/dev/null || true
  exit 1
fi

echo "  ${SUCCESS}✓${NC} Backend ready"
echo ""

# Start Vite dev server
echo "  ${MUTED}Starting Vite dev server on :5173...${NC}"
cd dashboard

if command -v bun >/dev/null 2>&1; then
  bun run dev &
else
  npm run dev &
fi
VITE_PID=$!

cd ..

# Wait for Vite
sleep 3

echo ""
echo "  ${SUCCESS}${BOLD}✓ Ready!${NC}"
echo ""
echo "  ${BOLD}Dashboard:${NC}  ${ACCENT}http://localhost:5173/dashboard/${NC}"
echo "  ${BOLD}Backend:${NC}    http://localhost:9867"
echo ""
echo "  ${MUTED}Edit dashboard/src/* and changes will hot-reload.${NC}"
echo "  ${MUTED}Press Ctrl+C to stop.${NC}"
echo ""

# Wait for either process to exit
wait $BACKEND_PID $VITE_PID
