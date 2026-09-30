#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
map_file="$script_dir/e2e-escalation.map"

suites=(api-extended cli-extended infra-extended smoke)

known_suite() {
  local candidate
  for candidate in "${suites[@]}"; do
    [ "$1" = "$candidate" ] && return 0
  done
  return 1
}

patterns=()
targets=()
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in '' | '#'*) continue ;; esac
  read -r pattern rule_suites <<<"$line"
  if [ -z "${rule_suites:-}" ]; then
    echo "detect-e2e-suites: rule names no suite: $line" >&2
    exit 1
  fi
  for suite in $rule_suites; do
    if [ "$suite" != "-" ] && ! known_suite "$suite"; then
      echo "detect-e2e-suites: unknown suite \"$suite\" in rule: $line" >&2
      exit 1
    fi
  done
  patterns+=("$pattern")
  targets+=("$rule_suites")
done <"$map_file"

if [ ${#patterns[@]} -eq 0 ]; then
  echo "detect-e2e-suites: $map_file holds no rule" >&2
  exit 1
fi

escalated=""
while IFS= read -r path || [ -n "$path" ]; do
  [ -n "$path" ] || continue
  for i in "${!patterns[@]}"; do
    if [[ $path =~ ${patterns[$i]} ]]; then
      escalated+=" ${targets[$i]} "
      break
    fi
  done
done

for suite in "${suites[@]}"; do
  if [[ $escalated == *" $suite "* ]]; then
    echo "run_${suite//-/_}=true"
  else
    echo "run_${suite//-/_}=false"
  fi
done

event="${E2E_EVENT:-pull_request}"
dispatch_suite="${E2E_DISPATCH_SUITE:-all}"
basic=()
extended=()
for area in api cli infra; do
  case "$event" in
    pull_request)
      basic+=("$area")
      [[ $escalated == *" $area-extended "* ]] && extended+=("$area-extended")
      ;;
    push)
      extended+=("$area-extended")
      ;;
    workflow_dispatch)
      if [ "$dispatch_suite" = all ] || [ "$dispatch_suite" = "$area" ]; then
        basic+=("$area")
        extended+=("$area-extended")
      fi
      ;;
  esac
done

json_list() {
  local out="" item
  for item in "$@"; do
    out+="${out:+,}\"$item\""
  done
  printf '[%s]' "$out"
}

echo "basic_suites=$(json_list ${basic[@]+"${basic[@]}"})"
echo "extended_suites=$(json_list ${extended[@]+"${extended[@]}"})"
