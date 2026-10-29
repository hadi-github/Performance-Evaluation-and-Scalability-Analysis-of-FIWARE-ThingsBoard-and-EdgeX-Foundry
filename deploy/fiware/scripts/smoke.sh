#!/usr/bin/env bash
# FIWARE smoke test: every replica healthy behind the LB; provision a service group + device;
# N UltraLight measures over HTTP must reach Orion.
# Limitation: Orion stores only the latest attribute value, so this checks reachability and ordering
# (HTTP is synchronous, so the final value must equal N), NOT exactly-once delivery. Exactly-once
# needs a notification sink counting Orion updates - see README.
cd "$(dirname "$0")/.." || exit 2
source ../../scripts/lib.sh
need curl jq

R="${FIWARE_REPLICAS:-1}"
SVC="${FIWARE_SERVICE:-openiot}"
ORION="http://${SUT_HOST}:${ORION_PORT:-1026}"
NORTH="http://${SUT_HOST}:${IOTA_NORTH_PORT:-4041}"
SOUTH="http://${SUT_HOST}:${IOTA_HTTP_PORT:-7896}"
H=(-H "fiware-service: ${SVC}" -H "fiware-servicepath: /")

wait_backend orion      "$R"
wait_backend iota-north "$R"
wait_backend iota-http  "$R"

KEY="smoke$(date +%s)"
DEV="dev-${KEY}"
ENT="urn:ngsi-ld:Sensor:${KEY}"

log "provisioning service group (apikey ${KEY}) and device ${DEV}"
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$NORTH/iot/services" "${H[@]}" -H 'Content-Type: application/json' \
  -d "{\"services\":[{\"apikey\":\"${KEY}\",\"entity_type\":\"Sensor\",\"resource\":\"/iot/d\"}]}")"
[[ "$code" == "201" ]] && pass "service group created" || fail "service group: HTTP ${code}"
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$NORTH/iot/devices" "${H[@]}" -H 'Content-Type: application/json' \
  -d "{\"devices\":[{\"device_id\":\"${DEV}\",\"apikey\":\"${KEY}\",\"entity_name\":\"${ENT}\",\"entity_type\":\"Sensor\",
       \"attributes\":[{\"object_id\":\"h\",\"name\":\"seqHttp\",\"type\":\"Integer\"}]}]}")"
[[ "$code" == "201" ]] && pass "device created" || fail "device: HTTP ${code}"

log "HTTP: sending ${SMOKE_N} UL measures through the LB"
err=0
for i in $(seq 1 "$SMOKE_N"); do
  code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$SOUTH/iot/d?k=${KEY}&i=${DEV}" -H 'Content-Type: text/plain' -d "h|$i")"
  [[ "$code" == "200" ]] || err=$((err+1))
done
(( err == 0 )) && pass "HTTP: all measures returned 200" || fail "HTTP: ${err} non-200 responses"

sleep 5
ent="$(curl -fsS "$ORION/v2/entities/${ENT}?options=keyValues" "${H[@]}")"
h="$(jq -r '.seqHttp // 0' <<<"$ent")"
[[ "$h" == "$SMOKE_N" ]] && pass "Orion seqHttp = ${h} (last HTTP measure arrived)" || fail "Orion seqHttp = ${h}, expected ${SMOKE_N}"

finish
