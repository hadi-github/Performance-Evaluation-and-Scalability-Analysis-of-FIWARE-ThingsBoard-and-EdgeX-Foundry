#!/usr/bin/env bash
# EdgeX smoke test: every replica healthy behind the LB, then N readings through device-rest must be
# persisted by core-data EXACTLY once (count == N catches both loss and the multi-replica duplicate bug).
cd "$(dirname "$0")/.." || exit 2
source ../../scripts/lib.sh
need curl jq

R="${EDGEX_REPLICAS:-1}"
META="http://${SUT_HOST}:${EDGEX_CORE_METADATA_PORT:-59881}/api/v3"
DATA="http://${SUT_HOST}:${EDGEX_CORE_DATA_PORT:-59880}/api/v3"
REST="http://${SUT_HOST}:${EDGEX_DEVICE_REST_PORT:-59986}/api/v3"

wait_backend core-metadata 1
wait_backend core-data   "$R"
wait_backend device-rest "$R"

# EdgeX returns HTTP 207 for batch calls; the real result is in each item's statusCode.
check207() { jq -e 'all(.[]; .statusCode == 201 or .statusCode == 409)' >/dev/null; }

log "provisioning profile bench-profile (idempotent)"
curl -fsS -X POST "$META/deviceprofile" -H 'Content-Type: application/json' -d '[{"apiVersion":"v3","profile":{
  "name":"bench-profile","manufacturer":"bench","model":"v1","labels":["bench"],
  "deviceResources":[{"name":"seq","isHidden":false,"properties":{"valueType":"Int64","readWrite":"RW"}}],
  "deviceCommands":[]}}]' | check207 && pass "profile ok" || fail "profile provisioning"

DEV="smoke-$(date +%s)"
log "provisioning device ${DEV}"
curl -fsS -X POST "$META/device" -H 'Content-Type: application/json' -d "[{\"apiVersion\":\"v3\",\"device\":{
  \"name\":\"${DEV}\",\"serviceName\":\"device-rest\",\"profileName\":\"bench-profile\",
  \"adminState\":\"UNLOCKED\",\"operatingState\":\"UP\",\"protocols\":{\"other\":{\"Address\":\"bench\"}}}}]" \
  | check207 && pass "device ok" || fail "device provisioning"

sleep 5   # let every device-rest replica receive the device-add system event over NATS

log "sending ${SMOKE_N} readings through the LB"
err=0
for i in $(seq 1 "$SMOKE_N"); do
  code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$REST/resource/${DEV}/seq" -H 'Content-Type: text/plain' -d "$i")"
  [[ "$code" == "200" ]] || err=$((err+1))
done
(( err == 0 )) && pass "all ingress requests returned 200" || fail "${err} non-200 ingress responses"

sleep 5   # device-rest -> NATS -> core-data -> Postgres is asynchronous
c="$(curl -fsS "$DATA/event/count/device/name/${DEV}" | jq -r '.count // .Count // 0')"; [[ "$c" =~ ^[0-9]+$ ]] || c=0
if   [[ "$c" == "$SMOKE_N" ]]; then pass "core-data persisted ${c}/${SMOKE_N} events (no loss, no duplicates)"
elif (( c > SMOKE_N ));       then fail "core-data persisted ${c}/${SMOKE_N}: DUPLICATES (queue group not effective?)"
else                               fail "core-data persisted ${c}/${SMOKE_N}: LOSS"; fi

finish
