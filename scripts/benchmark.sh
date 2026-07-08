#!/usr/bin/env bash
set -euo pipefail

DURATION="${1:-120}"
INTERVAL="${INTERVAL:-2}"
OUT="${OUT:-kioskmate-benchmark-$(date +%Y%m%d-%H%M%S).csv}"

echo "timestamp,load1,mem_used_mb,mem_available_mb,process,pid,cpu,mem,rss_kb,command" > "$OUT"
END=$((SECONDS + DURATION))

while [ "$SECONDS" -lt "$END" ]; do
  TS="$(date --iso-8601=seconds)"
  LOAD="$(awk '{print $1}' /proc/loadavg)"
  read -r MEM_USED MEM_AVAIL < <(free -m | awk '/Mem:/ {print $3, $7}')
  ps -eo pid=,pcpu=,pmem=,rss=,comm=,args= --sort=-pcpu |
    awk '/kioskmate|chromium|chrome/ {print; count++; if (count >= 12) exit}' |
    while read -r PID CPU MEM RSS COMM ARGS; do
      printf '%s,%s,%s,%s,%s,%s,%s,%s,%s,"%s"\n' "$TS" "$LOAD" "$MEM_USED" "$MEM_AVAIL" "$COMM" "$PID" "$CPU" "$MEM" "$RSS" "${ARGS//\"/\"\"}" >> "$OUT"
    done
  sleep "$INTERVAL"
done

echo "Wrote $OUT"
