#!/usr/bin/env bash
# ThingsBoard smoke test: every replica healthy behind the LB, then N messages over HTTP
# must be stored EXACTLY once (stored == sent detects both loss and duplication).
# Requires the demo tenant (TB_LOAD_DEMO=true at install time).
cd "$(dirname "$0")/.." || exit 2
source ../../scripts/lib.sh
need curl jq docker

R="${TB_REPLICAS:-1}"
BASE="http://${SUT_HOST}:${TB_HTTP_PORT:-8080}"

wait_backend tb-core           "$R"
wait_backend tb-http-transport "$R"
# The rule engine has no LB backend (it only consumes from Kafka): count its running containers instead.
re_up="$(docker compose ps --status running -q tb-rule-engine | wc -l)"
[[ "$re_up" == "$R" ]] && pass "tb-rule-engine: ${re_up}/${R} replicas running" || fail "tb-rule-engine: ${re_up}/${R} replicas running"

log "login as demo tenant"
JWT="$(curl -fsS -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"tenant@thingsboard.org","password":"tenant"}' | jq -r .token)"
AUTH=(-H "X-Authorization: Bearer ${JWT}")

DEV_NAME="smoke-$(date +%s)"
DEV_ID="$(curl -fsS -X POST "$BASE/api/device" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d "{\"name\":\"${DEV_NAME}\",\"type\":\"default\"}" | jq -r .id.id)"
TOKEN="$(curl -fsS "$BASE/api/device/${DEV_ID}/credentials" "${AUTH[@]}" | jq -r .credentialsId)"
log "device ${DEV_NAME} (${DEV_ID})"

# Explicit, distinct timestamps: ts_kv is keyed by (entity, key, ts), so server-assigned timestamps
# could collide within one millisecond and silently overwrite each other.
T0=$(( $(date +%s) * 1000 - 3600000 ))

log "HTTP: sending ${SMOKE_N} messages through the LB"
http_err=0
for i in $(seq 1 "$SMOKE_N"); do
  code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/${TOKEN}/telemetry" \
    -H 'Content-Type: application/json' -d "{\"ts\":$((T0+i)),\"values\":{\"seq_http\":$i}}")"
  [[ "$code" == "200" ]] || http_err=$((http_err+1))
done
(( http_err == 0 )) && pass "HTTP: all requests returned 200" || fail "HTTP: ${http_err} non-200 responses"

sleep 8   # SQL time-series writes are batched asynchronously

count() {
  curl -fsS "$BASE/api/plugins/telemetry/DEVICE/${DEV_ID}/values/timeseries?keys=$1&startTs=${T0}&endTs=$((T0+300000))&limit=1000000" \
    "${AUTH[@]}" | jq --arg k "$1" '.[$k] // [] | length'
}
c_http="$(count seq_http)"
[[ "$c_http" == "$SMOKE_N" ]] && pass "HTTP stored ${c_http}/${SMOKE_N}" || fail "HTTP stored ${c_http}/${SMOKE_N}"

finish
