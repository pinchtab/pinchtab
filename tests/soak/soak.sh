#!/usr/bin/env bash
# Stability soak: reuse the SAME browser across cycles under randomized mixed load
# (light / heavy / fixture pages), grow & churn tabs, periodically idle a few
# minutes then re-probe, and on unresponsiveness attempt a restart + record whether
# it recovered. Provision the container first with tests/soak/setup.sh.
#
# Usage:  CONTAINER=pinchtab-soak FIX_PORT=8088 tests/soak/soak.sh [DURATION_SECONDS]
set +e
CN="${CONTAINER:-pinchtab-soak}"
FIX="${FIX_PORT:-8088}"
DURATION="${1:-3600}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS="${RESULTS_DIR:-$HERE/results}"; mkdir -p "$RESULTS"
METRICS="$RESULTS/soak_metrics.csv"; SUMMARY="$RESULTS/soak_summary.txt"
START=$(date +%s); DEADLINE=$((START + DURATION))

LIGHT=(https://example.com "http://localhost:$FIX/article.html" "http://localhost:$FIX/form.html" "http://localhost:$FIX/qa.html")
HEAVY=("http://localhost:$FIX/heavy.html" https://en.wikipedia.org/wiki/Web_browser https://en.wikipedia.org/wiki/Operating_system https://news.ycombinator.com)
FIXP=(article.html form.html ecommerce.html dashboard.html spa.html serp.html directory.html accordion.html iframe.html editor.html autocomplete.html wizard.html)

timed(){ docker exec "$CN" sh -c 'S=$(date +%s%3N); { '"$1"' ; } >/dev/null 2>&1; rc=$?; E=$(date +%s%3N); echo "$((E-S))|$rc"'; }
ex(){ docker exec "$CN" sh -c "$1" 2>/dev/null; }
tabcount(){ ex 'pinchtab tab --json' | grep -o '"id"' | wc -l | tr -d ' '; }
cmem(){ docker stats --no-stream --format '{{.MemUsage}}' "$CN" 2>/dev/null | awk '{print $1}' | sed 's/MiB//;s/GiB/*1024/' | bc 2>/dev/null | cut -d. -f1; }
chromemem(){ ex "ps -o rss,comm 2>/dev/null | awk '/chrom/{s+=\$1} END{print int(s/1024)}'"; }

echo "ts,elapsed_s,cycle,phase,tabs,cmem_mb,chrome_mb,health_ms,health_ok,probe_ms,probe_ok,action,note" > "$METRICS"
cycle=0; fails=0; recoveries=0; failed_recoveries=0; peakmem=0; peaktabs=0; firstbreak=""
log(){ echo "$(date +%s),$(($(date +%s)-START)),$cycle,$1,$2,$3,$4,$5,$6,$7,$8,$9,${10}" >> "$METRICS"; }

echo "[soak] $CN for ${DURATION}s; metrics -> $METRICS"
while [ "$(date +%s)" -lt "$DEADLINE" ]; do
  cycle=$((cycle+1))
  IFS='|' read -r hms hrc <<<"$(timed 'pinchtab health')"
  IFS='|' read -r pms prc <<<"$(timed 'pinchtab title')"
  tabs=$(tabcount); [ -z "$tabs" ] && tabs=0
  cm=$(cmem); [ -z "$cm" ] && cm=0
  chm=$(chromemem); [ -z "$chm" ] && chm=0
  [ "${cm:-0}" -gt "$peakmem" ] 2>/dev/null && peakmem=$cm
  [ "${tabs:-0}" -gt "$peaktabs" ] 2>/dev/null && peaktabs=$tabs
  hok=$([ "$hrc" = 0 ] && echo 1 || echo 0); pok=$([ "$prc" = 0 ] && echo 1 || echo 0)
  unresp=0; { [ "$hrc" != 0 ] || [ "$prc" != 0 ] || [ "${pms:-99999}" -gt 20000 ]; } && unresp=1

  if [ "$unresp" = 1 ]; then
    fails=$((fails+1)); [ -z "$firstbreak" ] && firstbreak="cycle $cycle @ $(($(date +%s)-START))s (tabs=$tabs,mem=${cm}MB)"
    IFS='|' read -r rms rrc <<<"$(timed 'pinchtab server restart')"; sleep 6
    IFS='|' read -r r2ms r2rc <<<"$(timed 'pinchtab title')"
    if [ "$r2rc" = 0 ]; then recoveries=$((recoveries+1)); note="UNRESPONSIVE→RECOVERED(restart ${rms}ms,probe ${r2ms}ms)"; else failed_recoveries=$((failed_recoveries+1)); note="UNRESPONSIVE→RECOVERY-FAILED"; fi
    log "probe" "$tabs" "$cm" "$chm" "$hms" "$hok" "$pms" "$pok" "recover" "$note"; continue
  fi

  r=$((RANDOM % 12)); action=""; note=""
  if [ "$r" -lt 4 ]; then
    u=${HEAVY[$((RANDOM%${#HEAVY[@]}))]}; nt=$([ $((RANDOM%2)) = 0 ] && echo --new-tab || echo ""); action="nav-heavy $nt"
    IFS='|' read -r ams arc <<<"$(timed "pinchtab nav $u $nt")"; note="heavy ${ams}ms rc=$arc $u"
  elif [ "$r" -lt 7 ]; then
    u=${LIGHT[$((RANDOM%${#LIGHT[@]}))]}; action="nav-light"
    IFS='|' read -r ams arc <<<"$(timed "pinchtab nav $u")"; note="light ${ams}ms rc=$arc"
  elif [ "$r" -lt 9 ]; then
    f=${FIXP[$((RANDOM%${#FIXP[@]}))]}; action="nav-fixture --new-tab"
    IFS='|' read -r ams arc <<<"$(timed "pinchtab nav http://localhost:$FIX/$f --new-tab")"; note="fixture $f ${ams}ms rc=$arc"
  elif [ "$r" -lt 10 ]; then
    action="search"; IFS='|' read -r ams arc <<<"$(timed "pinchtab nav http://localhost:$FIX/serp.html && pinchtab text")"; note="search+extract ${ams}ms rc=$arc"
  elif [ "$r" -lt 11 ]; then
    if [ "$tabs" -gt 8 ]; then id=$(ex 'pinchtab tab --json' | grep -o '"id":"[^"]*"' | tail -1 | cut -d'"' -f4); ex "pinchtab close $id"; action="close-tab"; note="closed $id (tabs were $tabs)"; else action="extract"; IFS='|' read -r ams arc <<<"$(timed 'pinchtab snap')"; note="snap ${ams}ms rc=$arc"; fi
  else
    action="extract"; IFS='|' read -r ams arc <<<"$(timed 'pinchtab text')"; note="text ${ams}ms rc=$arc"
  fi
  log "load" "$tabs" "$cm" "$chm" "$hms" "$hok" "$pms" "$pok" "$action" "$note"

  if [ $((cycle % 7)) = 0 ]; then
    docker exec "$CN" python3 -c "import random;n=random.randint(8000,70000);open('/fixtures/heavy.html','w').write('<html><body><table>'+''.join(f'<tr><td>{i}</td><td>lorem {i}</td></tr>' for i in range(n))+'</table>'+'<div>x</div>'*random.randint(5000,30000)+'</body></html>')" 2>/dev/null
  fi
  if [ $((cycle % 10)) = 0 ]; then sleep $((120 + RANDOM % 120)); else sleep $((4 + RANDOM % 16)); fi
done

{
  echo "PinchTab soak summary"
  echo "duration: $(($(date +%s)-START))s   cycles: $cycle"
  echo "unresponsive events: $fails   recovered: $recoveries   recovery-failed: $failed_recoveries"
  echo "peak container mem: ${peakmem}MB   peak tabs: $peaktabs"
  echo "first breaking point: ${firstbreak:-none (stable for full run)}"
  echo "metrics: $METRICS"
} | tee "$SUMMARY"
