#!/bin/bash

# Use the most recent benchmark report
REPORT=$(ls -t results/benchmark_*.json 2>/dev/null | head -1)

if [[ -z "$REPORT" ]]; then
    echo "No baseline found"
    exit 1
fi

echo "Analyzing $REPORT"
jq '{
  type: .benchmark.type,
  run: .benchmark.run_number,
  timestamp: .benchmark.timestamp,
  passed: .totals.steps_passed,
  failed: .totals.steps_failed,
  total: (.totals.steps_passed + .totals.steps_failed)
}' "$REPORT"

# Count by group
echo ""
echo "Failures by group:"
jq '.steps[] | select(.pass == false) | .group' "$REPORT" | sort | uniq -c
