#!/usr/bin/env bash
# Run the outbound-observation sink on the load host, pinned to its core.
#   scripts/sink.sh start <out.ndjson> | scripts/sink.sh stop | scripts/sink.sh status
set -euo pipefail
cd "$(dirname "$0")/.."
# Deployment addresses come from .env through scripts/iotbench-env.sh; the
# fallbacks below keep the qualified values when it is absent.
# shellcheck source=scripts/iotbench-env.sh
source scripts/iotbench-env.sh
SUT_IP="${SUT_IP:-87.107.191.107}" PORT="${SINK_PORT:-18090}" CORE="${SINK_CPU:-2}"
SINK_BIN="${SINK_BIN:-bin/sink}"
PID_FILE="${SINK_PID_FILE:-/run/iot-bench-sink-${PORT}.pid}"
LOG_FILE="${SINK_LOG_FILE:-}"
RULE_STATE_FILE="${PID_FILE}.rule-added"
RULE_TAG=$(printf '%s' "$PID_FILE" | sha256sum | cut -c1-16)
RULE_COMMENT="iotbench-$RULE_TAG"
rule=(INPUT -p tcp --dport "$PORT" ! -i lo ! -s "$SUT_IP" -m comment --comment "$RULE_COMMENT" -j DROP)

matches_sink() {
  local pid=$1 expected_start=$2 expected_out=$3 actual_start
  local -a argv=()
  [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null || return 1
  actual_start=$(process_start_ticks "$pid")
  [[ "$actual_start" == "$expected_start" ]] || return 1
  mapfile -d '' -t argv < "/proc/$pid/cmdline" 2>/dev/null || return 1
  [[ ${#argv[@]} -ge 5 && ${argv[0]} == "$SINK_BIN" && ${argv[1]} == -addr && ${argv[2]} == "0.0.0.0:$PORT" && ${argv[3]} == -out && ${argv[4]} == "$expected_out" ]]
}

process_start_ticks() {
  [[ -r "/proc/$1/stat" ]] || return 1
  awk '{print $22}' "/proc/$1/stat"
}

process_running() {
  local state
  state=$(ps -p "$1" -o stat= 2>/dev/null || true)
  [[ -n "$state" && "$state" != Z* ]]
}

remove_owned_rule() {
  [[ -f "$RULE_STATE_FILE" ]] || return 0
  if [[ $(cat "$RULE_STATE_FILE" 2>/dev/null || true) == 1 ]]; then
    set +e
    iptables -w 5 -D "${rule[@]}" 2>/dev/null
    delete_rc=$?
    iptables -w 5 -C "${rule[@]}" 2>/dev/null
    check_rc=$?
    set -e
    if [[ $check_rc -eq 0 ]]; then
      echo "owned firewall rule still exists after one deletion; retaining $RULE_STATE_FILE" >&2
      return 1
    fi
    if [[ $check_rc -ne 1 ]]; then
      echo "cannot verify whether owned firewall rule remains (iptables check rc=$check_rc, delete rc=$delete_rc); retaining $RULE_STATE_FILE" >&2
      return 1
    fi
    rm -f "$RULE_STATE_FILE"
    return 0
  fi
  rm -f "$RULE_STATE_FILE"
}

case "${1:-}" in
  start)
    out="${2:?output file}"
    LOG_FILE="${LOG_FILE:-$out.log}"
    if [[ -e "$PID_FILE" ]]; then
      mapfile -t old_owner < "$PID_FILE" || true
      old_pid=${old_owner[0]:-}
      old_start=${old_owner[1]:-}
      old_out=${old_owner[2]:-}
      if matches_sink "$old_pid" "$old_start" "$old_out"; then
        echo "sink already running with pid $old_pid (pid file $PID_FILE)" >&2; exit 2
      fi
      if [[ "$old_pid" =~ ^[0-9]+$ ]] && kill -0 "$old_pid" 2>/dev/null; then
        echo "refusing stale pid file for unrelated live pid $old_pid" >&2; exit 2
      fi
      remove_owned_rule
      rm -f "$PID_FILE"
    elif [[ -e "$RULE_STATE_FILE" ]]; then
      remove_owned_rule
    fi
    if ss -Hlnpt "sport = :$PORT" 2>/dev/null | grep -q LISTEN; then
      echo "refusing occupied sink port $PORT" >&2; exit 2
    fi
    pid=""
    rollback_start() {
      rc=$?
      trap - EXIT
      if [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null; then
        kill -TERM "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
        if process_running "$pid"; then
          echo "startup rollback could not stop sink pid $pid; retaining pid/firewall ownership files" >&2
          exit 1
        fi
      fi
      rm -f "$PID_FILE"
      remove_owned_rule || exit 1
      exit "$rc"
    }
    trap rollback_start EXIT
    trap 'exit 143' TERM
    trap 'exit 130' INT
    set +e
    iptables -w 5 -C "${rule[@]}" >/dev/null 2>&1
    check_rc=$?
    set -e
    if [[ $check_rc -eq 0 ]]; then rule_added=0
    elif [[ $check_rc -eq 1 ]]; then rule_added=1
    else echo "cannot establish firewall-rule ownership (iptables check rc=$check_rc)" >&2; exit 1
    fi
    state_tmp="$RULE_STATE_FILE.tmp.$$"
    printf '%s\n' "$rule_added" > "$state_tmp"
    mv "$state_tmp" "$RULE_STATE_FILE"
    if [[ $rule_added -eq 1 ]]; then iptables -w 5 -I "${rule[@]}"; fi
    nohup taskset -c "$CORE" "$SINK_BIN" -addr "0.0.0.0:$PORT" -out "$out" > "$LOG_FILE" 2>&1 < /dev/null &
    pid=$!
    pid_start=$(process_start_ticks "$pid")
    pid_tmp="$PID_FILE.tmp.$$"
    printf '%s\n%s\n%s\n' "$pid" "$pid_start" "$out" > "$pid_tmp"
    mv "$pid_tmp" "$PID_FILE"
    for _ in $(seq 1 40); do
      if matches_sink "$pid" "$pid_start" "$out" && ss -Hlnpt "sport = :$PORT" 2>/dev/null | grep -Fq "pid=$pid,"; then
        curl -fsS "127.0.0.1:$PORT/metrics" | grep -E '^sink_(received|overflow)_total'
        trap - EXIT
        exit 0
      fi
      sleep 0.25
    done
    echo "sink readiness failed or listener is not owned by pid $pid on port $PORT" >&2
    exit 1 ;;
  stop)
    [[ -e "$PID_FILE" || -e "$RULE_STATE_FILE" ]] || { echo "no owned sink pid or firewall state for port $PORT" >&2; exit 2; }
    mapfile -t owner < "$PID_FILE" 2>/dev/null || true
    pid=${owner[0]:-}
    pid_start=${owner[1]:-}
    out=${owner[2]:-}
    LOG_FILE="${LOG_FILE:-${out:+$out.log}}"
    early_exit=false
    [[ -e "$PID_FILE" ]] || early_exit=true
    if [[ "$pid" =~ ^[0-9]+$ ]] && process_running "$pid"; then
      matches_sink "$pid" "$pid_start" "$out" || { echo "refusing to stop reused or unrelated pid $pid" >&2; exit 1; }
      kill -TERM "$pid"
      for _ in $(seq 1 120); do process_running "$pid" || break; sleep 0.25; done
      if process_running "$pid"; then echo "sink pid $pid did not stop within 30 seconds" >&2; exit 1; fi
    else
      early_exit=true
    fi
    shutdown_clean=false
    if [[ -n "$LOG_FILE" && -r "$LOG_FILE" ]] && grep -Eq 'shutdown_clean=true' "$LOG_FILE"; then shutdown_clean=true; fi
    rm -f "$PID_FILE"
    remove_owned_rule || exit 1
    echo "stopped pid=$pid shutdown_clean=$shutdown_clean early_exit=$early_exit"
    [[ "$shutdown_clean" == true && "$early_exit" == false ]] || exit 1 ;;
  verify)
    [[ -r "$PID_FILE" ]] || exit 1
    mapfile -t owner < "$PID_FILE"
    pid=${owner[0]:-}; pid_start=${owner[1]:-}; out=${owner[2]:-}
    matches_sink "$pid" "$pid_start" "$out" || exit 1
    ss -Hlnpt "sport = :$PORT" 2>/dev/null | grep -Fq "pid=$pid," ;;
  status)
    curl -fsS "127.0.0.1:$PORT/metrics" | grep -E '^sink_' || echo "not running" ;;
  *) sed -n '2,3p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
