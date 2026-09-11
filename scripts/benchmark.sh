#!/usr/bin/env bash
set -euo pipefail

DURATION="${1:-120}"
INTERVAL="${INTERVAL:-2}"
OUT="${OUT:-kioskmate-benchmark-$(date +%Y%m%d-%H%M%S).csv}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:33333/healthz}"

echo "timestamp,load1,mem_used_mb,mem_available_mb,browser_state,browser_ready,browser_cpu_pss,browser_rss_mb,browser_processes,renderer_cpu,renderer_rss_mb,gpu_cpu,gpu_rss_mb,navigation_state,load_ms,first_frame_ms,heartbeat_ms,duplicates,start_count,restart_count,recovery_state,process,pid,cpu,mem,rss_kb,command" > "$OUT"
END=$((SECONDS + DURATION))

while [ "$SECONDS" -lt "$END" ]; do
  TS="$(date --iso-8601=seconds)"
  LOAD="$(awk '{print $1}' /proc/loadavg)"
  read -r MEM_USED MEM_AVAIL < <(free -m | awk '/Mem:/ {print $3, $7}')
  API_FIELDS=",,,,,,,,,,,,,,,,,"
  if command -v curl >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
    HEALTH="$(curl --silent --show-error --max-time 2 "$HEALTH_URL" 2>/dev/null || true)"
    if [ -n "$HEALTH" ]; then
      API_FIELDS="$(printf '%s' "$HEALTH" | jq -r '[
        .browser.state, .browser.ready, .browser.process_tree.cpu_percent, .browser.process_tree.rss_mb,
        (.browser.process_tree.pids | length), .browser.process_tree.roles.renderer.cpu_percent,
        .browser.process_tree.roles.renderer.rss_mb, .browser.process_tree.roles.gpu.cpu_percent,
        .browser.process_tree.roles.gpu.rss_mb, .browser.navigation.state, .browser.navigation.load_duration_ms,
        .browser.navigation.first_frame_ms, .browser.navigation.heartbeat_latency_ms, .browser.duplicates,
        .browser.start_count, .browser.restart_count, .browser.recovery.state
      ] | map(. // "") | @csv' 2>/dev/null || printf ',,,,,,,,,,,,,,,,')"
    fi
  fi
  ps -eo pid=,pcpu=,pmem=,rss=,comm=,args= --sort=-pcpu |
    awk '/kioskmate|chromium|chrome/ {print; count++; if (count >= 12) exit}' |
    while read -r PID CPU MEM RSS COMM ARGS; do
      printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,"%s"\n' "$TS" "$LOAD" "$MEM_USED" "$MEM_AVAIL" "$API_FIELDS" "$COMM" "$PID" "$CPU" "$MEM" "$RSS" "${ARGS//\"/\"\"}" >> "$OUT"
    done
  sleep "$INTERVAL"
done

echo "Wrote $OUT"
