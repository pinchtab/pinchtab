#!/usr/bin/env bash
set -euo pipefail

# Simulate memory load by launching headless instances and filling them with tabs.
# Use alongside the monitoring dashboard to watch memory usage grow.
#
# Usage:
#   ./scripts/simulate-memory-load.sh [host:port] [instances] [tabs-per-instance]
#
# Examples:
#   ./scripts/simulate-memory-load.sh                      # 2 instances, 10 tabs each
#   ./scripts/simulate-memory-load.sh localhost:9867 3 20   # 3 instances, 20 tabs each
#
# Prerequisites:
#   - pinchtab running in server mode: ./pinchtab server
#   - Chrome installed

HOST="${1:-localhost:9867}"
NUM_INSTANCES="${2:-2}"
TABS_PER_INSTANCE="${3:-10}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

URLS=(
  "https://en.wikipedia.org/wiki/Main_Page"
  "https://news.ycombinator.com"
  "https://github.com/trending"
  "https://developer.mozilla.org/en-US/"
  "https://www.bbc.com/news"
  "https://stackoverflow.com/questions"
  "https://www.reddit.com/r/programming"
  "https://docs.github.com"
  "https://go.dev/doc/"
  "https://react.dev"
  "https://www.typescriptlang.org/docs/"
  "https://kubernetes.io/docs/home/"
  "https://www.rust-lang.org"
  "https://nodejs.org/en/docs"
  "https://www.postgresql.org/docs/"
)

api() {
  local method="$1" path="$2"
  shift 2
  curl -s -X "$method" "http://${HOST}${path}" \
    -H "Content-Type: application/json" \
    "$@"
}

echo -e "${CYAN}Memory Load Simulator${NC}"
echo -e "Target:              ${HOST}"
echo -e "Instances to launch: ${NUM_INSTANCES}"
echo -e "Tabs per instance:   ${TABS_PER_INSTANCE}"
echo ""

# Check server is up
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://${HOST}/health" 2>/dev/null || echo "000")
if [ "$STATUS" != "200" ]; then
  echo -e "${RED}✗ Server not reachable at ${HOST} (HTTP ${STATUS})${NC}"
  echo "  Start pinchtab first: ./pinchtab server"
  exit 1
fi
echo -e "${GREEN}✓ Server healthy${NC}"
echo ""

INSTANCE_IDS=()

# Phase 1: Launch instances
echo -e "${YELLOW}Phase 1: Launching ${NUM_INSTANCES} headless instances...${NC}"
for i in $(seq 1 "$NUM_INSTANCES"); do
  RESULT=$(api POST "/instances/launch" -d '{"mode":"headless"}')
  ID=$(echo "$RESULT" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

  if [ -z "$ID" ]; then
    echo -e "  ${RED}✗ Failed to launch instance ${i}: ${RESULT}${NC}"
    continue
  fi

  INSTANCE_IDS+=("$ID")
  echo -e "  ${GREEN}✓ Instance ${i}: ${ID}${NC}"
  sleep 1
done

if [ ${#INSTANCE_IDS[@]} -eq 0 ]; then
  echo -e "${RED}No instances launched. Exiting.${NC}"
  exit 1
fi

# Wait for instances to be ready
echo ""
echo -e "${YELLOW}Waiting for instances to initialize...${NC}"
sleep 3

# Phase 2: Open tabs
echo ""
echo -e "${YELLOW}Phase 2: Opening ${TABS_PER_INSTANCE} tabs per instance...${NC}"
for ID in "${INSTANCE_IDS[@]}"; do
  echo -e "  ${CYAN}Instance ${ID}:${NC}"
  for j in $(seq 1 "$TABS_PER_INSTANCE"); do
    URL_IDX=$(( (j - 1) % ${#URLS[@]} ))
    URL="${URLS[$URL_IDX]}"

    RESULT=$(api POST "/instances/${ID}/tabs/open" -d "{\"url\":\"${URL}\"}" 2>/dev/null)
    printf "\r    Opened %d/%d tabs" "$j" "$TABS_PER_INSTANCE"
    sleep 0.5
  done
  echo -e "\r    ${GREEN}✓ Opened ${TABS_PER_INSTANCE} tabs${NC}"
done

# Phase 3: Summary
echo ""
echo -e "${GREEN}Done!${NC}"
echo ""
echo -e "${CYAN}Monitor memory in the dashboard:${NC}"
echo -e "  1. Open http://${HOST} → Monitoring tab"
echo -e "  2. Enable 'Memory Metrics' in Settings"
echo -e "  3. Watch JS heap grow as pages load"
echo ""
echo -e "${CYAN}Check metrics via API:${NC}"
for ID in "${INSTANCE_IDS[@]}"; do
  echo -e "  curl http://${HOST}/instances/${ID}/tabs"
done
echo ""
echo -e "${CYAN}Cleanup — stop all instances:${NC}"
for ID in "${INSTANCE_IDS[@]}"; do
  echo -e "  curl -X POST http://${HOST}/instances/${ID}/stop"
done
