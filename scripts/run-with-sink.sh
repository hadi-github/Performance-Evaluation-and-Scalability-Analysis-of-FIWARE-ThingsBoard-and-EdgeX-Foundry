#!/usr/bin/env bash
# One run on the load host (ramonavm1): Go sink + benchctl run. The sink stays up until the platform stops
# exporting (received count unchanged for 30 s, at most 15 min) so a backlog is not truncated at the sink.
# In runs without a sink path the sink still runs and must receive nothing.
#
# Env: SINK_PORT (default 18090) must match scripts/sink.sh; SINK_HTTP is the local metrics port.
set -u
cd "$(dirname "$0")/.."
PLAN=$1; ID=$2; RES=$3
SINK_PORT="${SINK_PORT:-18090}"
mkdir -p "$RES"; rm -f /root/run.done
SINK_PORT="$SINK_PORT" scripts/sink.sh start "$RES/$ID.sink.ndjson" || { echo SINK-START-FAILED; exit 1; }
bin/benchctl run -plan "$PLAN" -results "$RES" -run-id "$ID" -cpuset 0-1 -env GOGC=400 \
  -prom-rw http://127.0.0.1:9090/api/v1/write > "$RES/$ID.run.txt" 2>&1
echo "benchctl rc=$?" >> "$RES/$ID.run.txt"
rcv() { curl -s "127.0.0.1:$SINK_PORT/metrics" | awk '/^sink_received_total/ {print $2}'; }
prev=-1; same=0; n=0
for _ in $(seq 1 90); do
  n=$(rcv); if [ "$n" = "$prev" ]; then same=$((same + 1)); else same=0; fi; prev=$n
  [ $same -ge 3 ] && break; sleep 10
done
echo "sink drain: received $n, stopped $(date -u +%T)" >> "$RES/$ID.run.txt"
# preserve the sink's own counters in the run evidence: recording loss (overflow) and unparsable
# deliveries must stay visible to the run audit, not just in the console
scripts/sink.sh status > "$RES/$ID.sink-metrics.txt" 2>&1 || true
scripts/sink.sh stop
echo "$ID" > /root/run.done
