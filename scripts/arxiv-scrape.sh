#!/usr/bin/env bash
# arxiv-scrape.sh — Scrape arXiv search results using pinchtab
#
# Usage: ./scripts/arxiv-scrape.sh [query] [max_pages]
# Example: ./scripts/arxiv-scrape.sh "ai agent memory" 3

set -euo pipefail

QUERY="${1:-ai agent memory}"
MAX_PAGES="${2:-3}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PT="${PINCHTAB_BIN:-$SCRIPT_DIR/../pinchtab-dev}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/arxiv-results}"
PARSER="$SCRIPT_DIR/parse_arxiv.py"

mkdir -p "$OUTPUT_DIR"
rm -f "$OUTPUT_DIR/papers.jsonl"

ENCODED_QUERY="${QUERY// /+}"
BASE_URL="https://arxiv.org/search/?query=${ENCODED_QUERY}&searchtype=all&resultsperpage=50"

echo "🔍 Searching arXiv for: \"$QUERY\"" >&2
echo "📄 Max pages: $MAX_PAGES" >&2
echo "📂 Output: $OUTPUT_DIR/" >&2
echo "" >&2

if ! "$PT" health &>/dev/null; then
  echo "❌ Pinchtab server not running." >&2
  exit 1
fi

TOTAL_PAPERS=0

for ((page=0; page<MAX_PAGES; page++)); do
  START=$((page * 50))
  URL="${BASE_URL}&start=${START}"

  echo "📖 Page $((page+1))/$MAX_PAGES (start=$START)..." >&2

  "$PT" nav "$URL" --block-images &>/dev/null
  sleep 2

  TMPFILE="$OUTPUT_DIR/.page_${page}.json"
  "$PT" text > "$TMPFILE" 2>/dev/null

  if grep -q "No results found" "$TMPFILE"; then
    echo "⚠️  No more results at page $((page+1))" >&2
    break
  fi

  PAGE_COUNT=$(python3 "$PARSER" parse "$TMPFILE" "$OUTPUT_DIR/papers.jsonl")
  TOTAL_PAPERS=$((TOTAL_PAPERS + PAGE_COUNT))
  echo "   Found $PAGE_COUNT papers (total: $TOTAL_PAPERS)" >&2
  rm -f "$TMPFILE"

  if ((page < MAX_PAGES - 1)); then
    sleep 1
  fi
done

echo "" >&2
echo "✅ Done! $TOTAL_PAPERS papers scraped" >&2

python3 "$PARSER" summary "$OUTPUT_DIR/papers.jsonl" "$OUTPUT_DIR"
