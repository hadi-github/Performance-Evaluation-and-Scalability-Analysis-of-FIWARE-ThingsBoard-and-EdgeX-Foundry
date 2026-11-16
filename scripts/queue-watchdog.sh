#!/usr/bin/env bash
# Keep the W1-W4 fixed-point queue running across workstation restarts.
#
# The campaign has twice lost its local launcher to a workstation reboot (ordinal
# 20 at 06:40Z and ordinal 25 at 15:17Z). Both times the traffic had already
# completed validly on the load host, but the local record was destroyed, which
# wastes a pre-registered ID and leaves a hole in the matrix. This watchdog makes
# that failure mode cheap instead of fatal: it re-arms the queue shortly after the
# machine comes back up, using whatever IDs still lack a terminal record.
#
# It is deliberately dumb and safe:
#   * it never launches, stops or modifies a run itself -- it only starts the
#     same driver under the same receipt;
#   * the driver skips every run_id that already has a terminal record and never
#     retries a spent ID, so re-arming is idempotent;
#   * it refuses to start a second driver if one is already alive;
#   * it refuses to start if any bound artefact has drifted from the receipt.
#
# Install once (as root, for the current user) with:
#   scripts/install-queue-watchdog.sh
set -uo pipefail
cd "$(dirname "$0")/.."
REPO=$PWD
log=/tmp/opencode/wmatrix-driver.log
mkdir -p /tmp/opencode

# Wait for the network and both servers before doing anything.
for _ in $(seq 1 60); do
  if ssh -F .iotbench-ssh-config -o BatchMode=yes -o ConnectTimeout=5 ramonavm2 true 2>/dev/null &&
     ssh -F .iotbench-ssh-config -o BatchMode=yes -o ConnectTimeout=5 ramonavm1 true 2>/dev/null; then
    break
  fi
  sleep 10
done

if pgrep -f 'drive-fixed-point-matrix\.py' >/dev/null 2>&1; then
  echo "=== watchdog $(date -u +%FT%TZ): driver already running; nothing to do ===" >> "$log"
  exit 0
fi

# Fail closed on receipt drift: a stale tree must not be launched unattended.
if ! IOTBENCH_ENV_FILE=/nonexistent python3 scripts/record-w-authorization.py \
       --receipt-id owner-w-matrix3-20261116h --check >/tmp/opencode/watchdog-check.log 2>&1; then
  echo "=== watchdog $(date -u +%FT%TZ): receipt check FAILED; not starting ===" >> "$log"
  cat /tmp/opencode/watchdog-check.log >> "$log"
  exit 1
fi

echo "=== watchdog $(date -u +%FT%TZ): arming the queue after a restart ===" >> "$log"
tmux new-session -d -s wmatrix -c "$REPO" \
  "python3 scripts/drive-fixed-point-matrix.py 2>&1 | tee -a $log; echo DRIVER_EXIT=\$? >> $log"

tmux new-session -d -s wsuper -c "$REPO" \
  "python3 scripts/supervise-fixed-point-queue.py 300 2>&1 | tee -a /tmp/opencode/wsuper.log"

tmux new-session -d -s tbsink -c "$REPO" \
  "bash scripts/ensure-tb-sink-route.sh 20"
