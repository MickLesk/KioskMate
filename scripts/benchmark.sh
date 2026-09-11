#!/usr/bin/env bash
set -euo pipefail

DURATION="${1:-120}"
INTERVAL="${INTERVAL:-2}"
OUT="${OUT:-kioskmate-benchmark-$(date +%Y%m%d-%H%M%S).csv}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:33333/healthz}"
REPORT="${REPORT:-${OUT%.csv}.json}"

echo "timestamp,load1,mem_used_mb,mem_available_mb,browser_state,browser_ready,browser_cpu_percent,browser_memory_mb,browser_processes,renderer_cpu,renderer_memory_mb,gpu_cpu,gpu_memory_mb,navigation_state,load_ms,first_frame_ms,heartbeat_ms,duplicates,start_count,restart_count,recovery_state,soak_status,soak_duration_seconds,soak_samples,soak_restarts,soak_duplicate_observed,process,pid,cpu,mem,rss_kb,command" > "$OUT"
END=$((SECONDS + DURATION))
EMPTY_API_FIELDS="$(printf '%*s' 21 '' | tr ' ' ',')"

while [ "$SECONDS" -lt "$END" ]; do
  TS="$(date --iso-8601=seconds)"
  LOAD="$(awk '{print $1}' /proc/loadavg)"
  read -r MEM_USED MEM_AVAIL < <(free -m | awk '/Mem:/ {print $3, $7}')
  API_FIELDS="$EMPTY_API_FIELDS"
  if command -v curl >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
    HEALTH="$(curl --silent --show-error --max-time 2 "$HEALTH_URL" 2>/dev/null || true)"
    if [ -n "$HEALTH" ]; then
      API_FIELDS="$(printf '%s' "$HEALTH" | jq -r '[
        .browser.state, .browser.ready, .browser.process_tree.cpu_percent, .browser.process_tree.rss_mb,
        (.browser.process_tree.pids | length), .browser.process_tree.roles.renderer.cpu_percent,
        .browser.process_tree.roles.renderer.rss_mb, .browser.process_tree.roles.gpu.cpu_percent,
        .browser.process_tree.roles.gpu.rss_mb, .browser.navigation.state, .browser.navigation.load_duration_ms,
        .browser.navigation.first_frame_ms, .browser.navigation.heartbeat_latency_ms, .browser.duplicates,
        .browser.start_count, .browser.restart_count, .browser.recovery.state,
        .soak.status, .soak.duration_seconds, .soak.samples, .soak.browser_restarts,
        .soak.duplicate_browser_observed
      ] | map(. // "") | @csv' 2>/dev/null || printf '%s' "$EMPTY_API_FIELDS")"
    fi
  fi
  ps -eo pid=,pcpu=,pmem=,rss=,comm=,args= --sort=-pcpu |
    awk 'tolower($0) ~ /kioskmate|chromium|chrome/ {print; count++; if (count >= 12) exit}' |
    while read -r PID CPU MEM RSS COMM ARGS; do
      printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,"%s"\n' "$TS" "$LOAD" "$MEM_USED" "$MEM_AVAIL" "$API_FIELDS" "$COMM" "$PID" "$CPU" "$MEM" "$RSS" "${ARGS//\"/\"\"}" >> "$OUT"
    done
  sleep "$INTERVAL"
done

echo "Wrote $OUT"

if command -v curl >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
  FINAL_HEALTH="$(curl --silent --show-error --max-time 5 "$HEALTH_URL" 2>/dev/null || true)"
  if [ -n "$FINAL_HEALTH" ] && printf '%s' "$FINAL_HEALTH" | jq -e . >/dev/null 2>&1; then
    ROWS=$(($(wc -l < "$OUT") - 1))
    CHECKSUM=""
    if command -v sha256sum >/dev/null 2>&1; then
      CHECKSUM="$(sha256sum "$OUT" | awk '{print $1}')"
    fi
    jq -n \
      --arg generated_at "$(date --iso-8601=seconds)" \
      --arg host "$(hostname)" \
      --arg kernel "$(uname -r)" \
      --arg architecture "$(uname -m)" \
      --arg csv "$OUT" \
      --arg csv_sha256 "$CHECKSUM" \
      --argjson duration_seconds "$DURATION" \
      --argjson interval_seconds "$INTERVAL" \
      --argjson rows "$ROWS" \
      --argjson health "$FINAL_HEALTH" \
      '{schema_version: 1, generated_at: $generated_at, host: $host, kernel: $kernel, architecture: $architecture, duration_seconds: $duration_seconds, interval_seconds: $interval_seconds, csv_rows: $rows, csv: $csv, csv_sha256: $csv_sha256, final_health: $health}' > "$REPORT"
    echo "Wrote $REPORT"
  else
    echo "Skipped JSON report: health endpoint did not return valid JSON" >&2
  fi
else
  echo "Skipped JSON report: curl and jq are required" >&2
fi
