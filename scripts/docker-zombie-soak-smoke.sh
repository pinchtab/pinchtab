#!/usr/bin/env bash
set -euo pipefail

IMAGE="${1:-pinchtab-local:test}"
SOAK_CYCLES="${SOAK_CYCLES:-5}"
SOAK_IDLE_SEC="${SOAK_IDLE_SEC:-180}"
IDLE_CHECK_SEC="${IDLE_CHECK_SEC:-15}"
RESTART_CYCLES="${RESTART_CYCLES:-3}"
INSTANCE_CYCLES="${INSTANCE_CYCLES:-3}"
TAB_CLOSE_SETTLE_SEC="${TAB_CLOSE_SETTLE_SEC:-2}"
FIXTURES=("form.html" "upload.html" "iframe.html" "toast.html" "wizard.html")
DRIFT_MID="${DRIFT_MID:-8}"
DRIFT_IDLE="${DRIFT_IDLE:-5}"
DRIFT_FINAL="${DRIFT_FINAL:-5}"
RESTART_SETTLE_SEC="${RESTART_SETTLE_SEC:-5}"
SMOKE_TOKEN="pinchtab-soak-${RANDOM}${RANDOM}"
NAME="pinchtab-soak-${RANDOM}${RANDOM}"
FIXTURE_PID=""
FAILED=0
RESULTS_DIR="tests/e2e/results/soak-$$"
HOST_PORT=""
FIXTURE_PORT=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SOAK_CONFIG="${REPO_ROOT}/tests/e2e/config/pinchtab-soak.json"

mkdir -p "$RESULTS_DIR"

ensure_image() {
  if [ "${SKIP_BUILD:-}" = "1" ] && docker image inspect "$IMAGE" >/dev/null 2>&1; then
    echo "Using existing local image: $IMAGE (SKIP_BUILD=1)"
    return 0
  fi

  echo "Building Docker image: $IMAGE ..."
  docker build -t "$IMAGE" .
}

cleanup() {
  if [ -n "$FIXTURE_PID" ] && kill -0 "$FIXTURE_PID" 2>/dev/null; then
    kill "$FIXTURE_PID" 2>/dev/null || true
  fi
  if [ -n "$NAME" ] && docker ps -a --format '{{.Names}}' | grep -Fxq "$NAME"; then
    snapshot_processes "shutdown_pre" >/dev/null || true
    if [ "$FAILED" -ne 0 ]; then
      echo ""
      echo "Container logs (last 100 lines):"
      docker logs --tail 100 "$NAME" 2>&1 || true
    fi
    docker rm -f "$NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

# ── Helpers ──────────────────────────────────────────────────────

timestamp() { date '+%Y-%m-%d %H:%M:%S'; }
log_phase() { echo ""; echo "[$(timestamp)] === $1 ==="; }

pt_curl() {
  local method="$1" path="$2"
  shift 2
  curl -sf \
    -X "$method" \
    -H "Authorization: Bearer $SMOKE_TOKEN" \
    -H "Content-Type: application/json" \
    "http://127.0.0.1:${HOST_PORT}${path}" \
    "$@" 2>/dev/null
}

pt_post() { pt_curl POST "$1" -d "$2"; }
pt_get()  { pt_curl GET "$1"; }
pt_del()  { pt_curl DELETE "$1" || true; }

PASS_COUNT=0
FAIL_COUNT=0
FAIL_LOG=""
FAIL_REASONS=()
PHASE_PASS=0
PHASE_FAIL=0
PHASE_WARNS=""

record_pass() { PASS_COUNT=$((PASS_COUNT + 1)); PHASE_PASS=$((PHASE_PASS + 1)); }
record_fail() {
  local label="$1"
  local reason="${2:-general}"
  FAIL_COUNT=$((FAIL_COUNT + 1))
  PHASE_FAIL=$((PHASE_FAIL + 1))
  FAIL_LOG="${FAIL_LOG}  - ${label}\n"
  PHASE_WARNS="${PHASE_WARNS}    ${label}\n"
  local already=0
  for r in "${FAIL_REASONS[@]+"${FAIL_REASONS[@]}"}"; do
    [ "$r" = "$reason" ] && already=1 && break
  done
  [ "$already" -eq 0 ] && FAIL_REASONS+=("$reason")
}

phase_summary() {
  local phase="$1" drift_tolerance="${2:-}"
  local z c delta verdict
  z=$(count_zombies)
  c=$(count_chrome)
  delta=$((c - BASELINE_CHROME))
  local health_ok="yes"
  pt_get /health >/dev/null 2>&1 || health_ok="no"
  local browser_ok="yes"
  pt_get /snapshot >/dev/null 2>&1 || browser_ok="no"

  if [ "$z" -gt 0 ] || [ "$health_ok" = "no" ] || [ "$PHASE_FAIL" -gt 0 ]; then
    verdict="FAIL"
  elif [ -n "$drift_tolerance" ] && [ "$delta" -gt "$drift_tolerance" ]; then
    verdict="WARN"
  else
    verdict="PASS"
  fi

  echo ""
  echo "  ┌─ ${phase} summary ─────────────────────────────"
  printf "  │ health: %-5s  browser: %-5s  zombies: %d\n" "$health_ok" "$browser_ok" "$z"
  printf "  │ chrome: %d (baseline: %d, drift: %+d)\n" "$c" "$BASELINE_CHROME" "$delta"
  printf "  │ assertions: %d passed, %d failed\n" "$PHASE_PASS" "$PHASE_FAIL"
  if [ -n "$PHASE_WARNS" ]; then
    echo -e "$PHASE_WARNS" | while IFS= read -r line; do
      [ -z "$line" ] && continue
      echo "  │ !! $line"
    done
  fi
  echo "  └─ verdict: ${verdict}"

  PHASE_PASS=0
  PHASE_FAIL=0
  PHASE_WARNS=""
}

run_step() {
  local label="$1"
  shift
  if "$@"; then
    record_pass
    return 0
  fi
  record_fail "$label" "general"
  FAILED=1
  return 1
}

navigate_url() {
  local url="$1"
  pt_post /navigate "{\"url\":\"${url}\"}" >/dev/null 2>&1
}

capture_screenshot() {
  pt_get "/screenshot?fullPage=false" >/dev/null 2>&1
}

fixture_url() {
  local path="$1"
  echo "http://host.docker.internal:${FIXTURE_PORT}/${path}"
}

count_zombies() {
  docker exec "$NAME" sh -c '
    z=0
    for f in /proc/[0-9]*/status; do
      [ -f "$f" ] || continue
      grep -q "^State:.*Z" "$f" 2>/dev/null && z=$((z + 1))
    done
    echo "$z"
  ' 2>/dev/null || echo 0
}

count_chrome() {
  docker exec "$NAME" sh -c '
    c=0
    for f in /proc/[0-9]*/cmdline; do
      [ -f "$f" ] || continue
      tr "\0" " " < "$f" 2>/dev/null | grep -qiE "chrom" && c=$((c + 1))
    done
    echo "$c"
  ' 2>/dev/null || echo 0
}

# Count chrome-family processes reparented to PID 1 (dumb-init) — orphan
# helpers whose original launcher exited without reaping them. crashpad_handler
# is excluded because Chrome intentionally daemonizes it under PID 1.
count_orphan_chrome_helpers() {
  docker exec "$NAME" sh -c '
    o=0
    for d in /proc/[0-9]*; do
      [ -f "$d/status" ] || continue
      [ -f "$d/cmdline" ] || continue
      cmdline=$(tr "\0" " " < "$d/cmdline" 2>/dev/null)
      [ -z "$cmdline" ] && continue
      printf "%s" "$cmdline" | grep -qiE "chrom|cloakbrowser" || continue
      printf "%s" "$cmdline" | grep -q "chrome_crashpad_handler" && continue
      name=$(awk "/^Name:/ {print \$2; exit}" "$d/status" 2>/dev/null)
      [ "$name" = "chrome_crashpad" ] && continue
      state=$(awk "/^State:/ {print \$2; exit}" "$d/status" 2>/dev/null)
      ppid=$(awk "/^PPid:/ {print \$2; exit}" "$d/status" 2>/dev/null)
      [ "$state" = "Z" ] && continue
      [ "$ppid" = "1" ] && o=$((o + 1))
    done
    echo "$o"
  ' 2>/dev/null || echo 0
}

count_active_instances() {
  local body
  body=$(pt_get /instances 2>/dev/null) || { echo ""; return 0; }
  echo "$body" | jq -r '[.[]? | select(.status=="running")] | length' 2>/dev/null || echo ""
}

snapshot_processes() {
  local label="${1:-snapshot}"
  local outfile="${RESULTS_DIR}/ps_$(date +%s)_${label}.txt"
  docker exec "$NAME" sh -c '
    echo "PID PPID STATE NAME"
    for d in /proc/[0-9]*; do
      [ -f "$d/status" ] || continue
      pid=${d##*/}
      name=$(grep "^Name:" "$d/status" 2>/dev/null | awk "{print \$2}")
      state=$(grep "^State:" "$d/status" 2>/dev/null | awk "{print \$2}")
      ppid=$(grep "^PPid:" "$d/status" 2>/dev/null | awk "{print \$2}")
      case "$name" in *pinchtab*|*chrom*|*dumb*) echo "$pid $ppid $state $name" ;; esac
    done
  ' 2>/dev/null | tee "$outfile"
}

check_zombies() {
  local label="$1"
  local z
  z=$(count_zombies)
  if [ "$z" -gt 0 ]; then
    echo "FAIL [$label]: $z zombie process(es)"
    snapshot_processes "zombie_${label}" >/dev/null
    record_fail "zombies: $label ($z found)" "zombie detected"
    FAILED=1
    return 1
  fi
  echo "OK   [$label]: 0 zombies"
  record_pass
  return 0
}

BASELINE_ORPHANS=0

# Asserts orphan-helper count hasn't grown beyond the post-warmup baseline.
# A non-zero baseline is normal: Chrome legitimately reparents some renderers/
# zygotes to init by design — we only flag growth, not steady state.
check_orphan_chrome() {
  local label="$1"
  local o
  o=$(count_orphan_chrome_helpers)
  local delta=$((o - BASELINE_ORPHANS))
  if [ "$delta" -gt 0 ]; then
    echo "FAIL [$label]: orphan chrome helpers grew +${delta} (baseline=$BASELINE_ORPHANS, current=$o)"
    snapshot_processes "orphan_${label}" >/dev/null
    record_fail "orphans: $label (+${delta})" "orphan chrome detected"
    FAILED=1
    return 1
  fi
  echo "OK   [$label]: orphan chrome helpers $o (baseline=$BASELINE_ORPHANS)"
  record_pass
  return 0
}

check_drift() {
  local label="$1" baseline="$2" tolerance="${3:-5}"
  local current
  current=$(count_chrome)
  local delta=$((current - baseline))
  if [ "$delta" -gt "$tolerance" ]; then
    echo "FAIL [$label]: chrome count drifted +${delta} (baseline=$baseline, current=$current)"
    record_fail "drift: $label (+$delta)" "drift exceeded"
    FAILED=1
    return 1
  fi
  echo "OK   [$label]: chrome count $current (baseline=$baseline, delta=$delta)"
  record_pass
  return 0
}

assert_health() {
  local label="$1"
  if pt_get /health >/dev/null 2>&1; then
    echo "OK   [$label]: health check passed"
    record_pass
    return 0
  fi
  echo "FAIL [$label]: health check failed"
  record_fail "health: $label" "health failure"
  FAILED=1
  return 1
}

assert_browser_responsive() {
  local label="$1"
  if pt_get /snapshot >/dev/null 2>&1; then
    echo "OK   [$label]: browser responsive"
    record_pass
    return 0
  fi
  echo "FAIL [$label]: browser unresponsive"
  record_fail "browser: $label" "browser unresponsive"
  FAILED=1
  return 1
}

capture_snapshot() {
  pt_get /snapshot >/dev/null 2>&1
}

run_fixture_workload() {
  local label="$1"
  local fixtures=("${FIXTURES[@]}")
  local ok=0 fail=0
  for fixture in "${fixtures[@]}"; do
    local url
    url=$(fixture_url "$fixture")
    if run_step "[$label] navigate $fixture" navigate_url "$url" 2>/dev/null; then
      ok=$((ok + 1))
    else
      fail=$((fail + 1))
    fi
    run_step "[$label] snapshot $fixture" capture_snapshot 2>/dev/null || true
    run_step "[$label] screenshot $fixture" capture_screenshot 2>/dev/null || true
  done
  # Inline zombie check after each workload batch
  local z
  z=$(count_zombies)
  if [ "$z" -gt 0 ]; then
    echo "  Workload [$label]: ${ok} navigated, ${fail} failed — ZOMBIES: $z"
    snapshot_processes "zombie_workload_${label}" >/dev/null
    record_fail "zombies mid-workload: $label ($z found)" "zombie detected"
    FAILED=1
  else
    echo "  Workload [$label]: ${ok} navigated, ${fail} failed — zombies: 0"
  fi
}

close_all_tabs() {
  local all_tabs closed=0
  all_tabs=$(pt_get /tabs 2>/dev/null | jq -r '.tabs[].id // empty' 2>/dev/null) || true
  if [ -n "$all_tabs" ]; then
    local tab_count
    tab_count=$(echo "$all_tabs" | wc -l | tr -d ' ')
    while IFS= read -r tid; do
      [ -z "$tid" ] && continue
      if [ "$tab_count" -le 1 ]; then break; fi
      pt_post "/tabs/${tid}/close" '{}' >/dev/null 2>&1 || true
      closed=$((closed + 1))
      tab_count=$((tab_count - 1))
    done <<< "$all_tabs"
  fi
  echo "$closed"
}

post_restart_probe() {
  local label="$1"
  # Navigate to a real page
  run_step "[$label] navigate" navigate_url "$(fixture_url "form.html")" || true
  # Snapshot the DOM
  run_step "[$label] snapshot" capture_snapshot || true
  # Screenshot
  run_step "[$label] screenshot" capture_screenshot || true
  # Open a new tab and close it
  local tab_resp tab_id
  tab_resp=$(pt_post /navigate "{\"url\":\"$(fixture_url "upload.html")\"}") || true
  tab_id=$(echo "$tab_resp" | jq -r '.tabId // empty' 2>/dev/null || true)
  if [ -n "$tab_id" ] && [ "$tab_id" != "null" ]; then
    run_step "[$label] open tab" true
    sleep 1
    if pt_post "/tabs/${tab_id}/close" '{}' >/dev/null 2>&1; then
      run_step "[$label] close tab" true
    else
      run_step "[$label] close tab" false || true
    fi
  else
    run_step "[$label] open tab" false || true
  fi
  sleep 1
  # Final snapshot after the churn settles
  run_step "[$label] post-settle snapshot" capture_snapshot || true
  check_zombies "${label}_probe" || true
}

idle_with_progress() {
  local total="$1"
  local step="$2"
  local label="$3"
  local elapsed=0
  while [ "$elapsed" -lt "$total" ]; do
    sleep "$step"
    elapsed=$((elapsed + step))
    local z c
    z=$(count_zombies)
    c=$(count_chrome)
    echo "  [$label] idle progress: ${elapsed}s/${total}s, zombies=$z, chrome=$c"
    snapshot_processes "${label}_${elapsed}s" >/dev/null || true
  done
}

start_container() {
  echo "Starting PinchTab container from $IMAGE..."
  docker run -d --name "$NAME" \
    -e PINCHTAB_TOKEN="$SMOKE_TOKEN" \
    -e PINCHTAB_CONFIG="/data/soak-config/pinchtab-soak.json" \
    -v "${SOAK_CONFIG}:/data/soak-config/pinchtab-soak.json:ro" \
    -p 127.0.0.1::9867 \
    --add-host=host.docker.internal:host-gateway \
    --shm-size=2g \
    "$IMAGE" >/dev/null

  HOST_PORT="$(docker port "$NAME" 9867/tcp | head -1 | awk -F: '{print $NF}')"
  if [ -z "$HOST_PORT" ]; then
    echo "FAIL: could not determine published host port"
    FAILED=1
    exit 1
  fi

  echo "Waiting for PinchTab on port $HOST_PORT..."
  for _ in $(seq 1 60); do
    if pt_get /health >/dev/null 2>&1; then break; fi
    sleep 1
  done

  if ! pt_get /health >/dev/null 2>&1; then
    echo "FAIL: health check did not pass within 60s"
    FAILED=1
    exit 1
  fi
  echo "PinchTab is healthy."

  echo "Warming up browser..."
  pt_post /navigate '{"url":"about:blank"}' >/dev/null 2>&1 || true
  for _ in $(seq 1 30); do
    if pt_get /snapshot >/dev/null 2>&1; then
      echo "Browser is ready."
      local z
      z=$(count_zombies)
      if [ "$z" -gt 0 ]; then
        echo "WARN: $z zombie(s) detected after browser startup"
        snapshot_processes "zombie_startup" >/dev/null || true
      fi
      return 0
    fi
    sleep 1
  done
  echo "WARN: browser warmup timed out (snapshot not responding after 30s)"
}

restart_container() {
  echo "Restarting PinchTab container..."
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  start_container
}

ensure_fixture_server() {
  if lsof -iTCP:8787 -sTCP:LISTEN >/dev/null 2>&1; then
    FIXTURE_PORT=8787
    echo "Using existing fixture server on port $FIXTURE_PORT"
    return 0
  fi

  if command -v python3 >/dev/null 2>&1; then
    echo "Starting local fixture server on port 8787..."
    (
      cd tests/tools/fixtures
      python3 -m http.server 8787 >/dev/null 2>&1
    ) &
    FIXTURE_PID=$!
    FIXTURE_PORT=8787
    for _ in $(seq 1 10); do
      if curl -sf "http://127.0.0.1:${FIXTURE_PORT}/" >/dev/null 2>&1; then
        return 0
      fi
      sleep 1
    done
    echo "WARN: fixture server did not become ready within 10s"
    return 0
  fi

  echo "FAIL: could not start fixture server (python3 missing)"
  FAILED=1
  exit 1
}

# ── Start container ──────────────────────────────────────────────

ensure_image
ensure_fixture_server
start_container

# ── Container metadata ──────────────────────────────────────────

echo "Capturing container metadata..."
docker inspect "$NAME" > "$RESULTS_DIR/docker_inspect.json" 2>/dev/null || true
docker top "$NAME" > "$RESULTS_DIR/docker_top.txt" 2>/dev/null || true
docker image inspect "$IMAGE" --format '{{.Id}} created={{.Created}}' > "$RESULTS_DIR/image_meta.txt" 2>/dev/null || true
docker exec "$NAME" sh -c 'pinchtab --version 2>/dev/null || echo "unknown"' > "$RESULTS_DIR/pinchtab_version.txt" 2>/dev/null || true

# ── Phase A: Baseline ───────────────────────────────────────────

log_phase "Phase A: Baseline capture"
echo "Expectations: healthy container, zero zombies, responsive browser, stable baseline process tree."
snapshot_processes "baseline"
BASELINE_CHROME=$(count_chrome)
BASELINE_ORPHANS=$(count_orphan_chrome_helpers)
echo "Baseline chrome process count: $BASELINE_CHROME"
echo "Baseline orphan helpers (chrome PPID=1, excl. crashpad): $BASELINE_ORPHANS"
check_zombies "baseline" || true
run_step "baseline navigate" navigate_url "$(fixture_url "form.html")" || true
sleep 1
phase_summary "Phase A: Baseline"

# ── Phase B: Fixture-backed browser churn ───────────────────────

log_phase "Phase B: Fixture-backed browser churn ($SOAK_CYCLES cycles)"
echo "Expectations: repeated real page work should not accumulate zombies or lose browser control."
for cycle in $(seq 1 "$SOAK_CYCLES"); do
  echo "  Cycle $cycle/$SOAK_CYCLES"
  run_fixture_workload "fixture_churn_${cycle}"

  TAB_IDS=()
  for fixture in "${FIXTURES[@]}"; do
    resp=$(pt_post /navigate "{\"url\":\"$(fixture_url "$fixture")\"}") || true
    if [ -n "$resp" ]; then
      tab_id=$(echo "$resp" | jq -r '.tabId // .id // empty' 2>/dev/null)
      [ -n "$tab_id" ] && [ "$tab_id" != "null" ] && TAB_IDS+=("$tab_id")
    fi
  done

  closed=$(close_all_tabs)
  echo "  Closed $closed tabs (keeping 1)"
  sleep "$TAB_CLOSE_SETTLE_SEC"

  snapshot_processes "fixture_churn_${cycle}" >/dev/null || true
  check_zombies "fixture_churn_${cycle}" || true
done

snapshot_processes "post_tab_churn" >/dev/null
check_drift "post_tab_churn" "$BASELINE_CHROME" "$DRIFT_MID" || true
phase_summary "Phase B: Tab churn" "$DRIFT_MID"

# ── Phase C: Instance lifecycle churn ─────────────────────────────────────

log_phase "Phase C: Instance lifecycle churn ($INSTANCE_CYCLES cycles)"
echo "Expectations: start/stop cycles should not leave zombie or drifting browser helpers behind."
for i in $(seq 1 "$INSTANCE_CYCLES"); do
  echo "  Instance cycle $i/$INSTANCE_CYCLES"
  resp=$(pt_post /instances/start '{"mode":"headless"}') || true
  inst_id=""
  if [ -n "$resp" ]; then
    inst_id=$(echo "$resp" | jq -r '.id // empty' 2>/dev/null)
  fi

  if [ -n "$inst_id" ] && [ "$inst_id" != "null" ] && [ "$inst_id" != "" ]; then
    for _ in $(seq 1 15); do
      status=$(pt_get "/instances/$inst_id" 2>/dev/null | jq -r '.status // empty' 2>/dev/null || true)
      [ "$status" = "running" ] && break
      sleep 1
    done

    run_step "instance ${inst_id} open tab" pt_post "/instances/$inst_id/tabs/open" "{\"url\":\"$(fixture_url "form.html")\"}" || true
    sleep 1

    run_step "instance ${inst_id} stop" pt_post "/instances/$inst_id/stop" '{}' || true

    for _ in $(seq 1 10); do
      status=$(pt_get "/instances/$inst_id" 2>/dev/null | jq -r '.status // empty' 2>/dev/null || true)
      case "$status" in stopped|""|null) break ;; esac
      sleep 1
    done
    echo "  OK: instance $inst_id created and stopped"
  else
    echo "  WARN: failed to launch instance $i"
  fi

  snapshot_processes "instance_churn_${i}" >/dev/null || true
  check_zombies "instance_churn_${i}" || true
  check_orphan_chrome "instance_churn_${i}" || true
done

snapshot_processes "post_instance_churn" >/dev/null
check_drift "post_instance_churn" "$BASELINE_CHROME" "$DRIFT_MID" || true
check_orphan_chrome "post_instance_churn" || true
phase_summary "Phase C: Instance churn" "$DRIFT_MID"

# ── Phase D: Restart / recovery churn ───────────────────────────

log_phase "Phase D: Restart / recovery churn (${RESTART_CYCLES} cycles)"
echo "Expectations: repeated restart + real work should preserve clean process state and browser responsiveness."
for i in $(seq 1 "$RESTART_CYCLES"); do
  echo "  Restart cycle $i/$RESTART_CYCLES"
  restart_container
  BASELINE_CHROME=$(count_chrome)
  BASELINE_ORPHANS=$(count_orphan_chrome_helpers)
  echo "  Re-baselined chrome count: $BASELINE_CHROME (orphans baseline: $BASELINE_ORPHANS)"
  post_restart_probe "restart_${i}"
  run_fixture_workload "restart_${i}"

  closed=$(close_all_tabs)
  echo "  Closed $closed tabs (keeping 1)"
  sleep "$TAB_CLOSE_SETTLE_SEC"

  snapshot_processes "restart_${i}" >/dev/null || true
  check_zombies "restart_${i}" || true
  check_drift "restart_${i}" "$BASELINE_CHROME" "$DRIFT_MID" || true
done
phase_summary "Phase D: Restart churn" "$DRIFT_MID"

# ── Phase E: Idle settle ────────────────────────────────────────

log_phase "Phase E: Idle settle (${SOAK_IDLE_SEC}s)"
echo "Expectations: no zombie growth, no uncontrolled process drift, browser still responsive after idle."
snapshot_processes "idle_start" >/dev/null
idle_with_progress "$SOAK_IDLE_SEC" "$IDLE_CHECK_SEC" "idle"
snapshot_processes "idle_end" >/dev/null

check_zombies "idle_settle" || true
check_drift "idle_settle" "$BASELINE_CHROME" "$DRIFT_IDLE" || true
run_step "idle settle navigate toast" navigate_url "$(fixture_url "toast.html")" || true
phase_summary "Phase E: Idle settle" "$DRIFT_IDLE"

# ── Phase F: Final verification ─────────────────────────────────

log_phase "Phase F: Final verification"
echo "Expectations: final zombie count zero, chrome drift bounded, service still healthy and controllable."
FINAL_CHROME=$(count_chrome)
FINAL_ZOMBIES=$(count_zombies)
FINAL_ORPHANS=$(count_orphan_chrome_helpers)

echo "Final chrome processes: $FINAL_CHROME (baseline: $BASELINE_CHROME)"
echo "Final zombie count: $FINAL_ZOMBIES"
echo "Final orphan helpers (chrome PPID=1, non-zombie): $FINAL_ORPHANS"

if [ "$FINAL_ZOMBIES" -gt 0 ]; then
  echo "FAIL [final]: zombie count $FINAL_ZOMBIES (must be 0)"
  record_fail "final: $FINAL_ZOMBIES zombies" "zombie detected"
  FAILED=1
else
  echo "OK   [final]: zero zombies"
  record_pass
fi

check_orphan_chrome "final" || true

CHROME_DELTA=$((FINAL_CHROME - BASELINE_CHROME))
if [ "$CHROME_DELTA" -gt "$DRIFT_FINAL" ]; then
  echo "FAIL [final]: chrome process drift +${CHROME_DELTA} above baseline"
  record_fail "final: chrome drift +${CHROME_DELTA}" "drift exceeded"
  FAILED=1
else
  echo "OK   [final]: chrome drift +${CHROME_DELTA} (acceptable)"
  record_pass
fi

snapshot_processes "final" >/dev/null
phase_summary "Phase F: Final" "$DRIFT_FINAL"

log_phase "Shutdown verification"
snapshot_processes "shutdown_pre" >/dev/null

echo ""
echo "╔══════════════════════════════════════════════════════╗"
echo "║                  SOAK TEST SUMMARY                  ║"
echo "╠══════════════════════════════════════════════════════╣"
printf "║  %-20s  %28s  ║\n" "Assertions passed:" "$PASS_COUNT"
printf "║  %-20s  %28s  ║\n" "Assertions failed:" "$FAIL_COUNT"
printf "║  %-20s  %28s  ║\n" "Final zombie count:" "$FINAL_ZOMBIES"
printf "║  %-20s  %28s  ║\n" "Final orphan helpers:" "$FINAL_ORPHANS"
printf "║  %-20s  %28s  ║\n" "Chrome baseline:" "$BASELINE_CHROME"
printf "║  %-20s  %28s  ║\n" "Chrome final:" "$FINAL_CHROME"
printf "║  %-20s  %28s  ║\n" "Chrome drift:" "${CHROME_DELTA}"
printf "║  %-20s  %28s  ║\n" "Process snapshots:" "$(ls "$RESULTS_DIR"/*.txt 2>/dev/null | wc -l | tr -d ' ')"
printf "║  %-20s  %28s  ║\n" "Artifacts:" "$RESULTS_DIR"
echo "╠══════════════════════════════════════════════════════╣"
if [ "${#FAIL_REASONS[@]}" -gt 0 ]; then
  echo "║  Failure reasons:                                   ║"
  for reason in "${FAIL_REASONS[@]}"; do
    printf "║    • %-48s║\n" "$reason"
  done
  echo "╠══════════════════════════════════════════════════════╣"
fi
if [ "$FAIL_COUNT" -gt 0 ]; then
  echo "║  Failed assertions:                                 ║"
  echo -e "$FAIL_LOG" | while IFS= read -r line; do
    [ -z "$line" ] && continue
    printf "║  %-52s║\n" "$line"
  done
  echo "╠══════════════════════════════════════════════════════╣"
fi
if [ "$FAILED" -ne 0 ]; then
  echo "║  RESULT: FAILED                                     ║"
  echo "╚══════════════════════════════════════════════════════╝"
  exit 1
fi
echo "║  RESULT: PASSED                                     ║"
echo "╚══════════════════════════════════════════════════════╝"
