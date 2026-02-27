#!/bin/bash

# =============================================================================
# Pinchtab Wrapper: Simple curl interface for common web extraction tasks
# =============================================================================
# 
# Purpose: Hide the two-step API workflow (create tab → get snapshot)
# behind a single, user-friendly interface.
#
# Why it exists: The Pinchtab API requires:
#   1. POST /tab {"action":"new","url":"..."} to get a tabId
#   2. GET /snapshot?tabId=<ID> to extract content
# This wrapper combines them so you can just: pinchtab-curl text <URL>
#
# Usage:
#   pinchtab-curl text <URL> [--format json|compact|text]
#   pinchtab-curl snapshot <URL> [--format json|compact|text] [--selector main]
#   pinchtab-curl click <URL> <ref> [--then-snapshot]
#   pinchtab-curl eval <URL> '<javascript>'
#   pinchtab-curl pdf <URL> [-o output.pdf]
#
# =============================================================================

set -e

# Configuration
PINCHTAB_HOST="${PINCHTAB_HOST:-localhost}"
PINCHTAB_PORT="${PINCHTAB_PORT:-9867}"
PINCHTAB_URL="http://$PINCHTAB_HOST:$PINCHTAB_PORT"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# =============================================================================
# Helper Functions
# =============================================================================

log_error() {
  echo -e "${RED}ERROR: $1${NC}" >&2
  exit 1
}

log_info() {
  echo -e "${GREEN}→${NC} $1"
}

log_debug() {
  if [ "$DEBUG" = "1" ]; then
    echo -e "${YELLOW}DEBUG: $1${NC}" >&2
  fi
}

# Create a new tab and navigate to URL, return tabId
create_tab() {
  local url=$1
  local timeout=${2:-10}
  
  log_info "Creating tab for: $url"
  
  local response=$(curl -s -X POST "$PINCHTAB_URL/tab" \
    -H "Content-Type: application/json" \
    -d "{\"action\":\"new\",\"url\":\"$url\",\"timeout\":$timeout}")
  
  local tab_id=$(echo "$response" | jq -r '.tabId // empty')
  
  if [ -z "$tab_id" ]; then
    log_error "Failed to create tab. Response: $response"
  fi
  
  log_debug "Created tab: $tab_id"
  echo "$tab_id"
}

# =============================================================================
# Task: Text Extraction (Cheapest, ~1K tokens)
# =============================================================================

cmd_text() {
  local url=$1
  local format=${2:-"text"} # or "raw"
  
  local tab_id=$(create_tab "$url")
  
  log_info "Extracting text from tab $tab_id"
  
  if [ "$format" = "raw" ]; then
    curl -s "$PINCHTAB_URL/text?tabId=$tab_id&mode=raw" | jq .
  else
    curl -s "$PINCHTAB_URL/text?tabId=$tab_id" | jq .
  fi
}

# =============================================================================
# Task: Snapshot (Accessibility Tree with Refs)
# =============================================================================

cmd_snapshot() {
  local url=$1
  local format=${2:-"compact"}
  local selector=${3:-""}
  
  local tab_id=$(create_tab "$url")
  
  log_info "Snapshotting tab $tab_id (format: $format)"
  
  local snapshot_url="$PINCHTAB_URL/snapshot?tabId=$tab_id&format=$format&maxTokens=2000"
  
  if [ -n "$selector" ]; then
    snapshot_url="$snapshot_url&selector=$selector"
  fi
  
  curl -s "$snapshot_url" | head -100
}

# =============================================================================
# Task: Click + Snapshot (Action-oriented)
# =============================================================================

cmd_click() {
  local url=$1
  local ref=$2
  local snapshot_after=${3:-true}
  
  local tab_id=$(create_tab "$url")
  
  log_info "Clicking ref $ref on tab $tab_id"
  
  curl -s -X POST "$PINCHTAB_URL/action" \
    -H "Content-Type: application/json" \
    -d "{\"kind\":\"click\",\"ref\":\"$ref\",\"tabId\":\"$tab_id\"}" | jq .
  
  if [ "$snapshot_after" = "true" ]; then
    log_info "Snapshotting after click"
    curl -s "$PINCHTAB_URL/snapshot?tabId=$tab_id&format=compact" | head -50
  fi
}

# =============================================================================
# Task: Evaluate JavaScript
# =============================================================================

cmd_eval() {
  local url=$1
  local expression=$2
  
  local tab_id=$(create_tab "$url")
  
  log_info "Evaluating: $expression"
  
  curl -s -X POST "$PINCHTAB_URL/evaluate" \
    -H "Content-Type: application/json" \
    -d "{\"expression\":\"$expression\",\"tabId\":\"$tab_id\"}" | jq .
}

# =============================================================================
# Task: PDF Export
# =============================================================================

cmd_pdf() {
  local url=$1
  local output_file=${2:-"output.pdf"}
  
  local tab_id=$(create_tab "$url")
  
  log_info "Exporting PDF to $output_file"
  
  curl -s "$PINCHTAB_URL/pdf?tabId=$tab_id&raw=true&output=file&path=$output_file"
  
  if [ -f "$output_file" ]; then
    log_info "PDF saved: $output_file"
  else
    log_error "Failed to save PDF"
  fi
}

# =============================================================================
# Task: Screenshot
# =============================================================================

cmd_screenshot() {
  local url=$1
  local output_file=${2:-"screenshot.jpg"}
  
  local tab_id=$(create_tab "$url")
  
  log_info "Taking screenshot to $output_file"
  
  curl -s "$PINCHTAB_URL/screenshot?tabId=$tab_id&raw=true" -o "$output_file"
  
  if [ -f "$output_file" ]; then
    log_info "Screenshot saved: $output_file"
  else
    log_error "Failed to save screenshot"
  fi
}

# =============================================================================
# Task: Multi-Step Workflow (Interactive)
# =============================================================================

cmd_workflow() {
  local url=$1
  
  local tab_id=$(create_tab "$url")
  
  log_info "Starting interactive workflow with tab $tab_id"
  log_info "Commands:"
  log_info "  snap              - snapshot current page"
  log_info "  click <ref>       - click element by ref"
  log_info "  type <ref> <text> - type into element"
  log_info "  eval <expr>       - evaluate JavaScript"
  log_info "  text              - extract text"
  log_info "  exit              - close and exit"
  echo ""
  
  while true; do
    read -p "pinchtab> " -a cmd_input
    
    case "${cmd_input[0]}" in
      snap)
        curl -s "$PINCHTAB_URL/snapshot?tabId=$tab_id&format=compact&maxTokens=1000" | head -50
        ;;
      click)
        if [ -z "${cmd_input[1]}" ]; then
          echo "Usage: click <ref>"
        else
          curl -s -X POST "$PINCHTAB_URL/action" \
            -H "Content-Type: application/json" \
            -d "{\"kind\":\"click\",\"ref\":\"${cmd_input[1]}\",\"tabId\":\"$tab_id\"}" | jq .
        fi
        ;;
      type)
        if [ -z "${cmd_input[1]}" ] || [ -z "${cmd_input[2]}" ]; then
          echo "Usage: type <ref> <text>"
        else
          curl -s -X POST "$PINCHTAB_URL/action" \
            -H "Content-Type: application/json" \
            -d "{\"kind\":\"type\",\"ref\":\"${cmd_input[1]}\",\"text\":\"${cmd_input[2]}\",\"tabId\":\"$tab_id\"}" | jq .
        fi
        ;;
      eval)
        if [ -z "${cmd_input[1]}" ]; then
          echo "Usage: eval <expression>"
        else
          curl -s -X POST "$PINCHTAB_URL/evaluate" \
            -H "Content-Type: application/json" \
            -d "{\"expression\":\"${cmd_input[1]}\",\"tabId\":\"$tab_id\"}" | jq .
        fi
        ;;
      text)
        curl -s "$PINCHTAB_URL/text?tabId=$tab_id" | jq .
        ;;
      exit)
        log_info "Closing tab and exiting"
        break
        ;;
      *)
        echo "Unknown command: ${cmd_input[0]}"
        ;;
    esac
  done
}

# =============================================================================
# Main
# =============================================================================

show_help() {
  cat << EOF
Pinchtab Wrapper: Simple curl interface for web extraction

USAGE:
  $(basename "$0") <command> <url> [options]

COMMANDS:
  text <url>                  Extract plain text (cheapest, ~2K tokens)
  snapshot <url> [format]     Get accessibility tree (compact|json|text)
  click <url> <ref>          Click element and optionally snapshot
  eval <url> '<expression>'   Run JavaScript on page
  pdf <url> [-o file.pdf]    Export page as PDF
  screenshot <url> [-o file]  Take screenshot
  workflow <url>              Interactive command loop (snap, click, type, eval, text)

OPTIONS:
  -h, --help                 Show this help
  --host <host>              Pinchtab host (default: localhost)
  --port <port>              Pinchtab port (default: 9867)

EXAMPLES:
  # Extract text from BBC News
  $(basename "$0") text https://www.bbc.com

  # Get interactive elements from CNN
  $(basename "$0") snapshot https://www.cnn.com compact

  # Click a link and see the result
  $(basename "$0") click https://www.example.com e5

  # Evaluate JavaScript
  $(basename "$0") eval https://www.example.com 'document.title'

  # Export page as PDF
  $(basename "$0") pdf https://www.example.com -o page.pdf

  # Start interactive session
  $(basename "$0") workflow https://www.example.com

ENVIRONMENT:
  PINCHTAB_HOST     (default: localhost)
  PINCHTAB_PORT     (default: 9867)
  DEBUG              Set to 1 for debug output

EOF
}

# Parse arguments
if [ $# -lt 2 ]; then
  show_help
  exit 1
fi

case "$1" in
  -h|--help)
    show_help
    exit 0
    ;;
  text)
    shift
    cmd_text "$@"
    ;;
  snapshot)
    shift
    cmd_snapshot "$@"
    ;;
  click)
    shift
    cmd_click "$@"
    ;;
  eval)
    shift
    cmd_eval "$@"
    ;;
  pdf)
    shift
    cmd_pdf "$@"
    ;;
  screenshot)
    shift
    cmd_screenshot "$@"
    ;;
  workflow)
    shift
    cmd_workflow "$@"
    ;;
  *)
    log_error "Unknown command: $1"
    ;;
esac
