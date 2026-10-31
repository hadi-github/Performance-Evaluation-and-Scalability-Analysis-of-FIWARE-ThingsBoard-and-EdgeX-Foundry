#!/usr/bin/env bash
# EdgeX setup procedure for every k (research/findings/edgex-device-validation-panic.md): adding a device while several
# device-rest replicas run makes core-metadata panic, so devices are provisioned at ONE replica and the deployment is
# scaled out afterwards (replicas load their devices from core-metadata at startup). Same procedure at every N.
#
#   deploy/edgex/scripts/provision-then-scale.sh prepare <N> <devices> <run_id> [gate.json]
#       resume-safe start (volumes preserved) at 1 replica on the final pool cpuset, create/reuse the profile and devices
#       bench-<run_id>-00000.., scale to N, start the export service (profile export), then the health gate:
#       every device-rest replica, addressed directly (not through the LB), accepts a reading for every device,
#       and core-metadata has zero restarts and zero panics. Exit 4 if the gate fails (run is INVALID: setup failure).
#       Gate readings carry run_id "<run_id>-gate" and seq -1, so no reconciliation counts them.
#   deploy/edgex/scripts/provision-then-scale.sh check <run_id> [gate.json] [check.json]
#       after the run: core-metadata restarts/panics compared with the gate. Exit 5 if any appeared: the run is
#       FAIL (platform defect); the evidence (log excerpt) is kept in check.json.
set -euo pipefail
cd "$(dirname "$0")/.."
META=http://127.0.0.1:59881/api/v3
PROFILE=bench-gen-v1   # workloads/k6/lib/adapters.js EDGEX_PROFILE

metadata_state() { # restarts and panics of core-metadata (logs span every restart of the container)
  local c=edgex-core-metadata-1
  jq -n --argjson r "$(docker inspect -f '{{.RestartCount}}' $c)" --arg s "$(docker inspect -f '{{.State.StartedAt}}' $c)" \
    --argjson p "$(docker logs $c 2>&1 | grep -c '^panic:' || true)" '{restarts: $r, started_at: $s, panics: $p}'
}

case "${1:-}" in
  prepare)
    N="${2:?N}"; D="${3:?devices}"; RUN="${4:?run_id}"; GATE="${5:-/root/expl/$RUN.gate.json}"
    if (( N == 1 )); then pool=4; else pool="4-$((3 + N))"; fi   # as scripts/resources.mk
    mkdir -p "$(dirname "$GATE")"
    export POOL_CPUSET="$pool"
    make -s check N="$N" >/dev/null
    # Never reset deployment data during qualification. Shrinking device-rest to
    # one replica is required before provisioning, but databases/volumes remain.
    EDGEX_REPLICAS=1 docker compose up -d
    until curl -sf -m3 -o /dev/null $META/ping && curl -sf -m3 -o /dev/null http://127.0.0.1:59986/api/v3/ping; do sleep 2; done

    echo "provisioning profile + $D devices at 1 replica"
    curl -fsS -o /dev/null -X POST $META/deviceprofile -H 'Content-Type: application/json' -d "[{\"apiVersion\":\"v3\",\"profile\":{
      \"name\":\"$PROFILE\",\"manufacturer\":\"bench\",\"model\":\"v1\",\"labels\":[\"bench\"],
      \"deviceResources\":[{\"name\":\"reading\",\"isHidden\":false,\"properties\":{\"valueType\":\"Object\",\"readWrite\":\"RW\",\"mediaType\":\"application/json\"}}],
      \"deviceCommands\":[]}}]"
    for ((off = 0; off < D; off += 100)); do
      body=$(jq -nc --arg run "$RUN" --argjson off $off --argjson n $(( D - off < 100 ? D - off : 100 )) --arg p $PROFILE \
        '[range($off; $off + $n) | {apiVersion: "v3", device: {name: ("bench-" + $run + "-" + ("0000" + tostring)[-5:]),
          serviceName: "device-rest", profileName: $p, adminState: "UNLOCKED", operatingState: "UP",
          protocols: {other: {Address: "bench"}}}}]')
      # right after start-up device-rest answers ping before it serves validation requests on the bus: retry
      # (409 = created by an earlier attempt; safe at one replica)
      for ((try = 1; ; try++)); do
        res=$(curl -sS -m 60 -X POST $META/device -H 'Content-Type: application/json' -d "$body" || true)
        jq -e 'type == "array" and length > 0 and all(.[]; .statusCode == 201 or .statusCode == 409)' \
          <<<"${res:-null}" >/dev/null 2>&1 && break
        (( try < 10 )) || { echo "device creation failed in batch at $off: $(head -c 300 <<<"$res")" >&2; exit 4; }
        sleep 3
      done
    done

    echo "scaling to N=$N, starting the export service"
    EDGEX_REPLICAS="$N" docker compose up -d --remove-orphans
    EDGEX_REPLICAS="$N" docker compose --profile export up -d app-http-export
    until [[ $(docker ps -q --filter name=edgex-device-rest --filter status=running | wc -l) == "$N" ]]; do sleep 2; done
    sleep 10   # device-rest replicas load devices from core-metadata at startup

    echo "gate: every replica accepts every device"
    replicas='[]'
    for c in $(docker ps --format '{{.Names}}' --filter name=edgex-device-rest | sort); do
      ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$c")
      until curl -sf -m3 -o /dev/null "http://$ip:59986/api/v3/ping"; do sleep 2; done
      ok=$(seq -f "bench-$RUN-%05g" 0 $((D - 1)) | xargs -P 16 -I{} curl -s -m 10 -o /dev/null -w '%{http_code}\n' \
        -X POST -H 'Content-Type: application/json' -d "{\"run_id\":\"$RUN-gate\",\"seq\":-1,\"scheduled_send_ms\":0}" \
        "http://$ip:59986/api/v3/resource/{}/reading" | grep -c '^200$' || true)
      replicas=$(jq -c --arg c "$c" --argjson ok "$ok" --argjson d "$D" '. + [{container: $c, devices_accepted: $ok, devices: $d}]' <<<"$replicas")
      echo "  $c: $ok/$D devices accepted"
    done
    meta=$(metadata_state)
    jq -n --arg run "$RUN" --argjson n "$N" --argjson d "$D" --arg pool "$pool" --argjson r "$replicas" --argjson m "$meta" \
      --arg t "$(date -u +%FT%TZ)" '{run_id: $run, n: $n, devices: $d, pool_cpuset: $pool, time_utc: $t,
        device_rest: $r, core_metadata: $m} |
      .reasons = ([.device_rest[] | select(.devices_accepted != .devices) | "\(.container) accepted \(.devices_accepted)/\(.devices) devices"]
        + (if .core_metadata.restarts > 0 or .core_metadata.panics > 0
           then ["core-metadata restarts \(.core_metadata.restarts), panics \(.core_metadata.panics)"] else [] end)) |
      .pass = (.reasons | length == 0)' > "$GATE"
    jq -c '{pass, reasons, core_metadata}' "$GATE"
    jq -e .pass "$GATE" >/dev/null || { echo "GATE FAILED: run is INVALID (setup failure)" >&2; exit 4; }
    ;;
  check)
    RUN="${2:?run_id}"; GATE="${3:-/root/expl/$RUN.gate.json}"; OUT="${4:-/root/expl/$RUN.check.json}"
    meta=$(metadata_state)
    jq -n --argjson g "$(jq .core_metadata "$GATE")" --argjson m "$meta" --arg t "$(date -u +%FT%TZ)" \
      --arg ev "$(docker logs edgex-core-metadata-1 2>&1 | grep -A8 '^panic:' | tail -40)" '
      {time_utc: $t, gate: $g, after: $m, new_restarts: ($m.restarts - $g.restarts), new_panics: ($m.panics - $g.panics)} |
      .defect = (.new_restarts > 0 or .new_panics > 0 or .after.started_at != .gate.started_at) |
      .verdict = (if .defect then "FAIL (platform defect: core-metadata panic/restart during the run)" else "no core-metadata defect" end) |
      .evidence = (if .defect then $ev else "" end)' > "$OUT"
    jq -c '{verdict, new_restarts, new_panics}' "$OUT"
    jq -e '.defect | not' "$OUT" >/dev/null || exit 5
    ;;
  *) sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
