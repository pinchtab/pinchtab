#!/usr/bin/env bash
set -euo pipefail

# Search for a keyword across major news sources using the Simple Strategy.
# Uses orchestrator shorthand endpoints (/navigate, /find) for scalability.
#
# Usage:
#   ./scripts/search-newspapers.sh [host:port] [keyword] [max-results]
#
# Examples:
#   ./scripts/search-newspapers.sh                          # Searches "trump" on 20 newspapers, 5 results each
#   ./scripts/search-newspapers.sh localhost:9867 "ai" 3    # Searches "ai", 3 results per source
#
# Prerequisites:
#   - pinchtab running: ./pinchtab
#   - Network access to news sites

HOST="${1:-localhost:9867}"
KEYWORD="${2:-trump}"
MAX_RESULTS="${3:-5}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BLUE='\033[0;34m'
NC='\033[0m'

# 20 major news sources
declare -a NEWSPAPERS=(
  "bbc.com/news"
  "cnn.com"
  "reuters.com"
  "apnews.com"
  "theguardian.com"
  "nytimes.com"
  "washingtonpost.com"
  "foxnews.com"
  "thehill.com"
  "politico.com"
  "npr.org"
  "bbc.co.uk"
  "dw.com"
  "aljazeera.com"
  "france24.com"
  "economist.com"
  "ft.com"
  "theatlantic.com"
  "axios.com"
  "businessinsider.com"
)

api() {
  local method="$1" path="$2"
  shift 2
  curl -s -X "$method" "http://${HOST}${path}" \
    -H "Content-Type: application/json" \
    "$@"
}

extract_field() {
  local json="$1" field="$2"
  echo "$json" | jq -r ".${field}?" 2>/dev/null || echo ""
}

echo -e "${CYAN}============================================${NC}"
echo -e "${CYAN}Newspaper Search - Simple Strategy Demo${NC}"
echo -e "${CYAN}============================================${NC}"
echo ""
echo -e "Target:     ${HOST}"
echo -e "Keyword:    ${KEYWORD}"
echo -e "Max Results: ${MAX_RESULTS} per source"
echo -e "Sources:    ${#NEWSPAPERS[@]} major news outlets"
echo ""

# Check server is up
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://${HOST}/health" 2>/dev/null || echo "000")
if [ "$STATUS" != "200" ]; then
  echo -e "${RED}✗ Server not reachable at ${HOST} (HTTP ${STATUS})${NC}"
  echo "  Start pinchtab first: ./pinchtab"
  exit 1
fi
echo -e "${GREEN}✓ Server healthy${NC}"
echo ""

# Results storage
declare -A RESULTS
TOTAL_MATCHES=0
SOURCES_SEARCHED=0
FAILED=0

# Search each newspaper
echo -e "${YELLOW}Searching ${#NEWSPAPERS[@]} sources for '${KEYWORD}'...${NC}"
echo ""

for SOURCE in "${NEWSPAPERS[@]}"; do
  URL="https://${SOURCE}"
  
  # Navigate to source
  NAV_RESULT=$(api POST "/navigate" -d "{\"url\":\"${URL}\"}" 2>/dev/null || echo "{}")
  NAV_CODE=$(echo "$NAV_RESULT" | jq -r '.code? // "error"' 2>/dev/null || echo "error")
  
  if [ "$NAV_CODE" = "error" ]; then
    echo -e "  ${RED}✗${NC} ${SOURCE}: Navigate failed"
    ((FAILED++))
    continue
  fi
  
  # Wait briefly for page load
  sleep 1
  
  # Search for keyword on current page
  FIND_RESULT=$(api POST "/find" -d "{\"text\":\"${KEYWORD}\"}" 2>/dev/null || echo "{}")
  MATCHES=$(echo "$FIND_RESULT" | jq ".refs? // []" 2>/dev/null | jq "length" 2>/dev/null || echo 0)
  
  if [ "$MATCHES" -gt 0 ]; then
    # Extract up to MAX_RESULTS matches
    TITLES=$(echo "$FIND_RESULT" | jq -r ".refs[0:${MAX_RESULTS}][] | .text?" 2>/dev/null | head -n "$MAX_RESULTS" || echo "")
    RESULTS["$SOURCE"]="$TITLES"
    echo -e "  ${GREEN}✓${NC} ${SOURCE}: ${MATCHES} matches (showing ${MAX_RESULTS})"
    TOTAL_MATCHES=$((TOTAL_MATCHES + MATCHES))
  else
    echo -e "  ${YELLOW}○${NC} ${SOURCE}: No matches"
  fi
  
  ((SOURCES_SEARCHED++))
done

echo ""
echo -e "${CYAN}============================================${NC}"
echo -e "${CYAN}Results Summary${NC}"
echo -e "${CYAN}============================================${NC}"
echo ""
echo -e "Sources searched: ${SOURCES_SEARCHED}/${#NEWSPAPERS[@]}"
echo -e "Failed searches:  ${FAILED}"
echo -e "Total matches:    ${TOTAL_MATCHES}"
echo ""

if [ $SOURCES_SEARCHED -eq 0 ]; then
  echo -e "${RED}No searches completed. Exiting.${NC}"
  exit 1
fi

# Display results
echo -e "${CYAN}Top Results for '${KEYWORD}':${NC}"
echo ""

for SOURCE in "${NEWSPAPERS[@]}"; do
  if [ -n "${RESULTS[$SOURCE]:-}" ]; then
    echo -e "${BLUE}${SOURCE}${NC}"
    echo "$RESULTS[$SOURCE]" | while IFS= read -r line; do
      if [ -n "$line" ]; then
        echo -e "  • ${line:0:80}"
      fi
    done
    echo ""
  fi
done

# Statistics
echo -e "${CYAN}============================================${NC}"
echo -e "${GREEN}Search complete!${NC}"
echo ""
echo -e "Performance:"
echo -e "  • Orchestrator used ${SOURCES_SEARCHED} instances"
echo -e "  • ${TOTAL_MATCHES} total '${KEYWORD}' mentions found"
echo -e "  • Simple strategy auto-allocated instances"
echo ""
