#!/usr/bin/env bash
# Run the outbound-observation sink on the load host (ramonavm1), pinned to its core, reachable only from the SUT.
#   scripts/sink.sh start <out.ndjson>   |   scripts/sink.sh stop   |   scripts/sink.sh status
set -euo pipefail
cd "$(dirname "$0")/.."
SUT_IP="${SUT_IP:-87.107.191.107}" PORT="${SINK_PORT:-18090}" CORE="${SINK_CPU:-2}"
rule=(INPUT -p tcp --dport "$PORT" ! -s "$SUT_IP" ! -s 127.0.0.1 -m comment --comment iot-bench-sink -j DROP)
case "${1:-}" in
  start)
    out="${2:?output file}"
    iptables -C "${rule[@]}" 2>/dev/null || iptables -I "${rule[@]}"
    pgrep -f "^bin/sink " >/dev/null && { echo "sink already running" >&2; exit 2; }
    nohup taskset -c "$CORE" bin/sink -addr "0.0.0.0:$PORT" -out "$out" > "$out.log" 2>&1 < /dev/null &
    sleep 1; curl -fsS "127.0.0.1:$PORT/metrics" | grep -E "^sink_(received|overflow)_total" ;;
  stop)
    pkill -TERM -f "^bin/sink " && sleep 2 || true
    while iptables -D "${rule[@]}" 2>/dev/null; do :; done; echo stopped ;;
  status) curl -fsS "127.0.0.1:$PORT/metrics" | grep -E "^sink_" || echo "not running" ;;
  *) sed -n '2,3p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
