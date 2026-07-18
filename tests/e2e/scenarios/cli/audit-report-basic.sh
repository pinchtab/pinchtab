#!/bin/bash
# audit-report-basic.sh — pinchtab audit --format md/html rendered reports.

GROUP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${GROUP_DIR}/../../helpers/cli.sh"

OUT_DIR=$(mktemp -d /tmp/audit-report.XXXXXX)

# ─────────────────────────────────────────────────────────────────
start_test "audit --format md writes report.md next to report.json"

pt_ok audit "${FIXTURES_URL}/audit-site/broken-assets.html" --output-dir "$OUT_DIR" --format md

if [ -f "$OUT_DIR/report.json" ]; then
  pass_assert "report.json exists"
else
  fail_assert "report.json exists"
fi
if [ -f "$OUT_DIR/report.md" ]; then
  pass_assert "report.md exists"
else
  fail_assert "report.md exists"
fi

for heading in "## Summary" "## Broken Assets"; do
  if grep -q "$heading" "$OUT_DIR/report.md" 2>/dev/null; then
    pass_assert "report.md contains '$heading'"
  else
    fail_assert "report.md contains '$heading'"
  fi
done

end_test

# ─────────────────────────────────────────────────────────────────
start_test "audit --format html writes a self-contained report.html"

pt_ok audit "${FIXTURES_URL}/audit-site/broken-assets.html" --output-dir "$OUT_DIR" --format html

if [ -f "$OUT_DIR/report.html" ]; then
  pass_assert "report.html exists"
else
  fail_assert "report.html exists"
fi
if grep -q "<style>" "$OUT_DIR/report.html" 2>/dev/null; then
  pass_assert "report.html inlines its CSS"
else
  fail_assert "report.html inlines its CSS"
fi
if grep -q 'src="screenshots/' "$OUT_DIR/report.html" 2>/dev/null; then
  pass_assert "report.html links screenshots relatively"
else
  fail_assert "report.html links screenshots relatively"
fi

end_test

rm -rf "$OUT_DIR"
