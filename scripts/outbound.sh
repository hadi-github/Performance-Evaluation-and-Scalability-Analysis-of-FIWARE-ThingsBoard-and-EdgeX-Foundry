#!/usr/bin/env bash
# Configure each platform's outbound delivery to the sink (docs/methodology/exploratory-capacity.md, "Setup").
# This is setup required by the measurement (research/notes/decisions.md: telemetry semantics), not tuning.
#
#   scripts/outbound.sh thingsboard   REST API Call node after "Save Timeseries" -> Success in the root rule chain
#   scripts/outbound.sh thingsboard-remove   remove that node (observer-overhead runs without the sink path)
#   ThingsBoard: with EVIDENCE=<file>, the root rule chain metadata after the change is written there.
#   scripts/outbound.sh fiware        Orion subscription on all Sensor entities -> sink (keyValues)
#   scripts/outbound.sh edgex         nothing to configure here: the app-service-configurable export service
#                                     (deploy/edgex/compose.yaml profile "export") carries the sink URL
# Env: SUT (default 87.107.191.107), SINK_URL (default http://87.107.191.32:18090), TB_USER/TB_PASS.
set -euo pipefail
SUT="${SUT:-87.107.191.107}"
SINK="${SINK_URL:-http://87.107.191.32:18090}"

case "${1:-}" in
  thingsboard|thingsboard-remove)
    TB="http://$SUT:8080"
    jwt=$(curl -fsS -X POST "$TB/api/auth/login" -H 'Content-Type: application/json' \
      -d "{\"username\":\"${TB_USER:-tenant@thingsboard.org}\",\"password\":\"${TB_PASS:-tenant}\"}" | jq -r .token)
    H=(-H "X-Authorization: Bearer $jwt" -H 'Content-Type: application/json')
    root=$(curl -fsS "${H[@]}" "$TB/api/ruleChains?pageSize=50&page=0&type=CORE" | jq -r '.data[] | select(.root) | .id.id')
    meta=$(curl -fsS "${H[@]}" "$TB/api/ruleChain/$root/metadata")
    if [[ "$1" == thingsboard-remove ]]; then
      idx=$(jq '[.nodes[].name] | index("bench sink")' <<<"$meta")
      [[ "$idx" != null ]] || { echo "no 'bench sink' node"; exit 0; }
      new=$(jq --argjson i "$idx" '.nodes |= del(.[$i])
        | .connections |= [.[] | select(.toIndex != $i and .fromIndex != $i)
                              | .toIndex -= (if .toIndex > $i then 1 else 0 end)
                              | .fromIndex -= (if .fromIndex > $i then 1 else 0 end)]' <<<"$meta")
      curl -fsS "${H[@]}" -X POST "$TB/api/ruleChain/metadata" -d "$new" | jq -c '{nodes: [.nodes[].name], connections: (.connections | length)}'
      [[ -z "${EVIDENCE:-}" ]] || curl -fsS "${H[@]}" "$TB/api/ruleChain/$root/metadata" > "$EVIDENCE"
      exit 0
    fi
    if jq -e '.nodes[] | select(.name == "bench sink")' <<<"$meta" >/dev/null; then
      echo "root rule chain already has the 'bench sink' node"; exit 0
    fi
    save=$(jq '[.nodes[].type] | index("org.thingsboard.rule.engine.telemetry.TbMsgTimeseriesNode")' <<<"$meta")
    [[ "$save" != null ]] || { echo "no Save Timeseries node in the root rule chain" >&2; exit 1; }
    new=$(jq --argjson save "$save" --arg url "$SINK/tb" '
      .nodes += [{
        "type": "org.thingsboard.rule.engine.rest.TbRestApiCallNode", "name": "bench sink", "debugMode": false,
        "configuration": {
          "restEndpointUrlPattern": $url, "requestMethod": "POST", "useSimpleClientHttpFactory": false,
          "parseToPlainText": false, "ignoreRequestBody": false, "enableProxy": false, "useSystemProxyProperties": false,
          "readTimeoutMs": 10000, "maxParallelRequestsCount": 0, "headers": {"Content-Type": "application/json"},
          "credentials": {"type": "anonymous"}}}]
      | .connections += [{"fromIndex": $save, "toIndex": (.nodes | length - 1), "type": "Success"}]' <<<"$meta")
    curl -fsS "${H[@]}" -X POST "$TB/api/ruleChain/metadata" -d "$new" | jq -c '{nodes: [.nodes[].name], connections: (.connections | length)}'
    [[ -z "${EVIDENCE:-}" ]] || curl -fsS "${H[@]}" "$TB/api/ruleChain/$root/metadata" > "$EVIDENCE"
    ;;
  fiware)
    ORION="http://$SUT:1026"
    FH=(-H "fiware-service: ${FIWARE_SERVICE:-openiot}" -H 'fiware-servicepath: /')
    subscriptions=$(curl -fsS "${FH[@]}" "$ORION/v2/subscriptions")
    count=$(jq '[.[] | select(.description == "bench sink")] | length' <<<"$subscriptions")
    if (( count > 1 )); then
      echo "multiple bench sink subscriptions; refusing ambiguous route" >&2; exit 1
    fi
    if (( count == 1 )); then
      current=$(jq -r '.[] | select(.description == "bench sink") | .notification.http.url' <<<"$subscriptions")
      if [[ "$current" == "$SINK/fiware" ]]; then
        echo "subscription 'bench sink' already has the requested route"; exit 0
      fi
      id=$(jq -r '.[] | select(.description == "bench sink") | .id' <<<"$subscriptions")
      body=$(jq -nc --arg url "$SINK/fiware" '{notification:{http:{url:$url},attrs:["seq","run_id","scheduled_send_ms"],attrsFormat:"keyValues"}}')
      curl -fsS -X PATCH "${FH[@]}" -H 'Content-Type: application/json' \
        "$ORION/v2/subscriptions/$id" --data-binary "$body"
      echo "updated bench sink route; waiting 65 s for replica cache refresh"; sleep 65
      exit 0
    fi
    curl -fsS -o /dev/null -w "subscription: HTTP %{http_code}\n" "${FH[@]}" -H 'Content-Type: application/json' \
      -X POST "$ORION/v2/subscriptions" -d "{
        \"description\": \"bench sink\",
        \"subject\": {\"entities\": [{\"idPattern\": \".*\", \"type\": \"Sensor\"}], \"condition\": {\"attrs\": [\"seq\"]}},
        \"notification\": {\"http\": {\"url\": \"$SINK/fiware\"}, \"attrs\": [\"seq\", \"run_id\", \"scheduled_send_ms\"],
                          \"attrsFormat\": \"keyValues\"}}"
    # Each Orion replica learns the subscription on its next subscription-cache refresh (-subCacheIval, default
    # 60 s); before that only the replica that created it notifies (EXPL-fiware-trace: 50 of 200 at N=4).
    echo "waiting 65 s for every Orion replica to refresh its subscription cache"; sleep 65
    ;;
  edgex) echo "EdgeX: start the export service: (cd edgex && docker compose --profile export up -d app-http-export)" ;;
  *) sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
