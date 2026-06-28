#!/usr/bin/env bash
# Escalated burst: grow tabs UNBOUNDED with heavy pages to find the browser's
# breaking point and exercise recovery. Provision with a tight cap and a high tab
# limit first, e.g.:  MEM_CAP=1g SHM=256m MAX_TABS=200 tests/soak/setup.sh
#
# Usage:  CONTAINER=pinchtab-soak FIX_PORT=8088 tests/soak/burst.sh [DURATION_SECONDS]
set +e
CN="${CONTAINER:-pinchtab-soak}"
FIX="${FIX_PORT:-8088}"
DURATION="${1:-1200}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS="${RESULTS_DIR:-$HERE/results}"; mkdir -p "$RESULTS"
METRICS="$RESULTS/burst_metrics.csv"; SUMMARY="$RESULTS/burst_summary.txt"
START=$(date +%s); DEADLINE=$((START + DURATION))

HEAVY=("http://localhost:$FIX/heavy.html" https://en.wikipedia.org/wiki/Operating_system "http://localhost:$FIX/ecommerce.html" "http://localhost:$FIX/dashboard.html")

timed(){ docker exec "$CN" sh -c 'S=$(date +%s%3N); { '"$1"' ; } >/dev/null 2>&1; rc=$?; E=$(date +%s%3N); echo "$((E-S))|$rc"'; }
ex(){ docker exec "$CN" sh -c "$1" 2>/dev/null; }
tabcount(){ ex 'pinchtab tab --json' | grep -o '"id"' | wc -l | tr -d ' '; }
cmem(){ docker stats --no-stream --format '{{.MemUsage}}' "$CN" 2>/dev/null | awk '{print $1}' | sed 's/MiB//;s/GiB/*1024/' | bc 2>/dev/null | cut -d. -f1; }
chromemem(){ ex "ps -o rss,comm 2>/dev/null | awk '/chrom/{s+=\$1} END{print int(s/1024)}'"; }
oomflag(){ docker inspect "$CN" --format '{{.State.OOMKilled}}' 2>/dev/null; }

echo "ts,elapsed_s,cycle,tabs,cmem_mb,chrome_mb,health_ms,health_ok,probe_ms,probe_ok,nav_ms,nav_ok,event" > "$METRICS"
cycle=0; breaks=0; recoveries=0; failed_rec=0; peakmem=0; peaktabs=0; firstbreak=""

echo "[burst] $CN for ${DURATION}s; metrics -> $METRICS"
while [ "$(date +%s)" -lt "$DEADLINE" ]; do
  cycle=$((cycle+1))
  IFS='|' read -r hms hrc <<<"$(timed 'pinchtab health')"
  IFS='|' read -r pms prc <<<"$(timed 'pinchtab title')"
  tabs=$(tabcount); [ -z "$tabs" ] && tabs=0
  cm=$(cmem); [ -z "$cm" ] && cm=0; chm=$(chromemem); [ -z "$chm" ] && chm=0
  [ "${cm:-0}" -gt "$peakmem" ] 2>/dev/null && peakmem=$cm
  [ "${tabs:-0}" -gt "$peaktabs" ] 2>/dev/null && peaktabs=$tabs
  hok=$([ "$hrc" = 0 ] && echo 1 || echo 0); pok=$([ "$prc" = 0 ] && echo 1 || echo 0)

  u=${HEAVY[$((RANDOM%${#HEAVY[@]}))]}
  IFS='|' read -r nms nrc <<<"$(timed "pinchtab nav $u --new-tab")"
  nok=$([ "$nrc" = 0 ] && echo 1 || echo 0)

  broke=0; { [ "$hrc" != 0 ] || [ "$prc" != 0 ] || [ "${pms:-99999}" -gt 20000 ] || [ "$nrc" != 0 ]; } && broke=1
  if [ "$broke" = 1 ]; then
    breaks=$((breaks+1)); oom=$(oomflag)
    [ -z "$firstbreak" ] && firstbreak="cycle $cycle @ $(($(date +%s)-START))s: tabs=$tabs mem=${cm}MB chrome=${chm}MB oom=$oom (health_rc=$hrc title_rc=$prc title_ms=$pms nav_rc=$nrc)"
    echo "$(date +%s),$(($(date +%s)-START)),$cycle,$tabs,$cm,$chm,$hms,$hok,$pms,$pok,$nms,$nok,BREAK(oom=$oom)" >> "$METRICS"
    IFS='|' read -r rms rrc <<<"$(timed 'pinchtab server restart')"; sleep 8
    IFS='|' read -r r2ms r2rc <<<"$(timed "pinchtab nav http://localhost:$FIX/article.html")"
    if [ "$r2rc" = 0 ]; then recoveries=$((recoveries+1)); ev="RECOVERED(restart ${rms}ms, nav ${r2ms}ms)"; else failed_rec=$((failed_rec+1)); ev="RECOVERY-FAILED(rc=$r2rc)"; fi
    echo "$(date +%s),$(($(date +%s)-START)),$cycle,$(tabcount),$(cmem),$(chromemem),-,-,-,-,-,-,$ev" >> "$METRICS"
    sleep 2; continue
  fi
  echo "$(date +%s),$(($(date +%s)-START)),$cycle,$tabs,$cm,$chm,$hms,$hok,$pms,$pok,$nms,$nok,grow" >> "$METRICS"
  sleep 1
done

{
  echo "PinchTab BURST (escalated) summary"
  echo "duration: $(($(date +%s)-START))s   cycles: $cycle"
  echo "break events: $breaks   recovered: $recoveries   recovery-failed: $failed_rec"
  echo "peak mem: ${peakmem}MB   peak tabs: $peaktabs"
  echo "FIRST BREAKING POINT: ${firstbreak:-none (survived full run)}"
} | tee "$SUMMARY"
