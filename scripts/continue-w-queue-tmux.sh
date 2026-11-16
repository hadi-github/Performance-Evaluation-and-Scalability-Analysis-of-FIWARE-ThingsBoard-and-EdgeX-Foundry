#!/usr/bin/env bash
# Wait for the current driver invocation to finish, then run whatever ordinals of
# the authorized queue are still without a terminal record.
#
# The driver reads its queue once at start-up. Ordinal 60 (W4/thingsboard
# repetition 5) was authorized after the current invocation had already loaded its
# list, so that process will exit when it exhausts ordinals 22-55. This script
# keeps the campaign hands-off: it does not launch, stop or modify any run itself,
# it only starts the same driver again under the new receipt.
#
# Resumption is safe because the driver skips every run_id that already has a
# terminal record in driver-log.jsonl, and it never retries a spent ID.
set -uo pipefail
cd "$(dirname "$0")/.."
target_pid=${1:?current driver pid required}
log=/tmp/opencode/wmatrix-driver.log

while kill -0 "$target_pid" 2>/dev/null; do
  sleep 60
done

echo "=== continuation: previous driver $target_pid exited at $(date -u +%FT%TZ) ===" >> "$log"

# Let the previous session's own shell finish writing its exit marker.
while tmux has-session -t wmatrix 2>/dev/null; do
  sleep 5
done

tmux new-session -d -s wmatrix -c "$PWD" \
  "python3 scripts/drive-fixed-point-matrix.py --start-ordinal 60 2>&1 | tee -a $log; echo DRIVER_EXIT=\$? >> $log"

echo "=== continuation: started driver for remaining ordinals at $(date -u +%FT%TZ) ===" >> "$log"
