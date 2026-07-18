#!/bin/bash
# audit-pdf-extended.sh — pinchtab audit --format pdf export.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

OUT_DIR=$(mktemp -d /tmp/audit-pdf.XXXXXX)

# ─────────────────────────────────────────────────────────────────
start_test "audit --format pdf writes a real report.pdf"

pt_ok audit "http://fixtures/audit-site/index.html" --output-dir "$OUT_DIR" --format pdf

if [ -f "$OUT_DIR/report.json" ]; then
  pass_assert "report.json exists"
else
  fail_assert "report.json exists"
fi

PDF="$OUT_DIR/report.pdf"
if [ -f "$PDF" ]; then
  pass_assert "report.pdf exists"
else
  fail_assert "report.pdf exists"
fi

MAGIC=$(head -c 4 "$PDF" 2>/dev/null)
if [ "$MAGIC" = "%PDF" ]; then
  pass_assert "report.pdf starts with %PDF magic bytes"
else
  fail_assert "report.pdf starts with %PDF magic bytes (got: $MAGIC)"
fi

SIZE=$(wc -c < "$PDF" | tr -d ' ')
if [ "$SIZE" -gt 10240 ]; then
  pass_assert "report.pdf is > 10KB ($SIZE bytes)"
else
  fail_assert "report.pdf is > 10KB (got: $SIZE bytes)"
fi

end_test

rm -rf "$OUT_DIR"
