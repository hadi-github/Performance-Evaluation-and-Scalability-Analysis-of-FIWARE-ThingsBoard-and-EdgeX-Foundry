#!/usr/bin/env bash
# Shared helpers for the per-platform smoke tests. Requires: curl, jq, docker.
set -euo pipefail

# Load ./.env (or .env.example) without overriding variables already set in the environment,
# so `TB_REPLICAS=3 ./scripts/smoke.sh` wins over the file - same precedence docker compose uses.
load_env() {
  local f=.env line k
  [[ -f "$f" ]] || f=.env.example
  [[ -f "$f" ]] || return 0
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[[:space:]]*# || ! "$line" =~ = ]] && continue
    k="${line%%=*}"; k="${k//[[:space:]]/}"
    [[ -n "${!k+x}" ]] || export "${k}=${line#*=}"
  done < "$f"
}
load_env

SUT_HOST="${SUT_HOST:-localhost}"
LB_STATS_PORT="${LB_STATS_PORT:-8404}"
SMOKE_N="${SMOKE_N:-200}"

log()  { printf '\033[1;34m[smoke]\033[0m %s\n' "$*"; }
pass() { printf '\033[1;32m[PASS]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[FAIL]\033[0m %s\n' "$*"; FAILED=1; }
FAILED=0

need() { for b in "$@"; do command -v "$b" >/dev/null || { echo "missing dependency: $b" >&2; exit 2; }; done; }

# Count servers in an HAProxy backend whose status is UP.
backend_up() {
  curl -fsS "http://${SUT_HOST}:${LB_STATS_PORT}/stats;csv" \
    | awk -F, -v b="$1" '$1==b && $2!="BACKEND" && $2!="FRONTEND" && $18 ~ /^UP/ {n++} END {print n+0}'
}

# Wait until backend $1 has exactly $2 UP servers (each replica answered the LB health check).
wait_backend() {
  local backend="$1" want="$2" timeout="${3:-${WAIT_TIMEOUT:-300}}" got=0 t=0
  log "waiting for ${want} healthy replica(s) behind '${backend}'"
  while (( t < timeout )); do
    got="$(backend_up "$backend" 2>/dev/null || echo 0)"
    if [[ "$got" == "$want" ]]; then pass "${backend}: ${got}/${want} replicas UP"; return 0; fi
    sleep 3; t=$((t+3))
  done
  fail "${backend}: ${got}/${want} replicas UP after ${timeout}s"
  return 0
}

finish() {
  if [[ "$FAILED" == "0" ]]; then pass "all smoke checks passed"; else fail "smoke test FAILED"; exit 1; fi
}
