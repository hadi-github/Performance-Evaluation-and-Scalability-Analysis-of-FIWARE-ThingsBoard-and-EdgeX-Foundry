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
#
# Revision 2 (2026-11-10) — repairs the ordinal-11 zero-traffic setup failure
# (research/findings/edgex-r11-second-replica-readiness-failure.md). Behavioural changes, all
# confined to SETUP (before any measurement traffic):
#   * every wait loop is bounded; no unbounded `until ... sleep` can hang the launcher;
#   * a replica that never exposes its ingest route is restarted ONCE, bounded
#     (EDGEX_REPLICA_REPAIR_RESTARTS, default 1), and re-gated;
#   * any setup failure writes a diagnostics dump (<gate>.failure.txt) with container
#     state, restart counts and log tails, so a failure is self-explaining.
# Unchanged: provisioning at one replica, the not-a-device readiness probe, the inventory
# gate, the gate evidence files, exit codes and the "never reset data" rule.
set -euo pipefail
cd "$(dirname "$0")/.."
META=http://127.0.0.1:59881/api/v3
PROFILE=bench-gen-v1   # workloads/k6/lib/adapters.js EDGEX_PROFILE

PING_TIMEOUT="${EDGEX_PING_TIMEOUT:-120}"
SCALE_TIMEOUT="${EDGEX_SCALE_TIMEOUT:-180}"
REPAIR_RESTARTS="${EDGEX_REPLICA_REPAIR_RESTARTS:-1}"
# A non-numeric or negative budget would abort later with a bare "unbound
# variable"; fail closed at parse time instead.
[[ "$REPAIR_RESTARTS" =~ ^[0-9]+$ ]] || {
  echo "EDGEX_REPLICA_REPAIR_RESTARTS must be a non-negative integer, got '$REPAIR_RESTARTS'" >&2
  exit 2
}

# Bounded wait for a device-rest replica's HTTP ping. $1=base $2=label(optional)
wait_http_ping() {
  local base=$1 label=${2:-$1} deadline=$((SECONDS + PING_TIMEOUT))
  while (( SECONDS < deadline )); do
    curl -sf -m3 -o /dev/null "$base/api/v3/ping" && return 0
    sleep 2
  done
  echo "device-rest HTTP ping did not answer within ${PING_TIMEOUT}s: $label ($base)" >&2
  return 4
}

wait_driver_ready() {
  # v4.0.2 starts HTTP ping and metadata callbacks before InitCache finishes.
  # The POST-only resource route is added by driver.Start AFTER bootstrap/cache
  # initialization. GET must return 405, not startup 404/503. No event is sent.
  # Default read inside the function so the extracted-function unit test is
  # self-contained (tests/edgex_driver_ready_test.py slices this text only).
  local base=$1 timeout=${2:-${EDGEX_DRIVER_READY_TIMEOUT:-180}} status last=none deadline
  deadline=$((SECONDS + timeout))
  while (( SECONDS < deadline )); do
    status=$(curl -s -m 3 -o /dev/null -w '%{http_code}' \
      "$base/api/v3/resource/__bench_readiness__/reading" || true)
    [[ "$status" == 405 ]] && return 0
    last=$status
    sleep 1
  done
  echo "device-rest ingest route not initialized before readiness deadline (${timeout}s): $base (last status: $last)" >&2
  return 4
}

metadata_state() { # restarts and panics of core-metadata (logs span every restart of the container)
  local c=edgex-core-metadata-1
  jq -n --argjson r "$(docker inspect -f '{{.RestartCount}}' $c)" --arg s "$(docker inspect -f '{{.State.StartedAt}}' $c)" \
    --argjson p "$(docker logs $c 2>&1 | grep -c '^panic:' || true)" '{restarts: $r, started_at: $s, panics: $p}'
}

# Self-explaining setup failure: container state + restart counts + log tails.
# $1 = output path, $2 = one-line reason
setup_failure_dump() {
  local out=$1 reason=$2
  {
    echo "# EdgeX setup failure $(date -u +%FT%TZ)"
    echo "reason: $reason"
    echo
    echo "## containers"
    docker ps -a --format '{{.Names}}\t{{.Status}}' | grep -E '^edgex-' || true
    echo
    for c in $(docker ps -a --format '{{.Names}}' --filter name=edgex-device-rest | sort); do
      echo "## inspect $c"
      docker inspect -f 'name={{.Name}} started={{.State.StartedAt}} exit={{.State.ExitCode}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} ip={{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$c" || true
      echo "## logs $c (last 80)"
      docker logs --tail 80 "$c" 2>&1 || true
      echo
    done
    echo "## core-metadata"
    docker inspect -f 'started={{.State.StartedAt}} restarts={{.RestartCount}} exit={{.State.ExitCode}}' edgex-core-metadata-1 || true
    docker logs --tail 200 edgex-core-metadata-1 2>&1 | grep -E '^panic:|WARN|ERROR' | tail -20 || true
    echo
    echo "## core-keeper"
    docker logs --tail 200 edgex-core-keeper-1 2>&1 | grep -E 'WARN|ERROR|misbehaving' | tail -20 || true
    echo
    echo "## core-data"
    for c in $(docker ps -a --format '{{.Names}}' --filter name=edgex-core-data | sort); do
      docker inspect -f 'name={{.Name}} restarts={{.RestartCount}} exit={{.State.ExitCode}}' "$c" || true
    done
  } > "$out" 2>&1 || true
}

REPAIRS_USED=0   # total bounded replica restarts performed by this prepare run

# Bounded, bounded-count repair for a replica that never exposes its ingest route.
# device-rest is stateless and reloads its device cache from core-metadata at startup,
# so a restart re-attempts bootstrap from scratch, which is exactly what the ordinal-11
# replica needed and never got. $1=container. The replica IP is re-derived after each
# restart because a recreated container may land on a different bridge address.
ensure_replica_ready() {
  local c=$1 base=${2:-} ip attempt
  wait_driver_ready "$base" && return 0
  for (( attempt = 1; attempt <= REPAIR_RESTARTS; attempt++ )); do
    echo "replica $c did not expose its ingest route; bounded restart $attempt/$REPAIR_RESTARTS" >&2
    REPAIRS_USED=$((REPAIRS_USED + 1))
    docker restart "$c" >/dev/null || return 4
    ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$c" || true)
    [[ -n "$ip" ]] || return 4
    base="http://$ip:59986"
    wait_http_ping "$base" "$c (after restart $attempt)" || return 4
    wait_driver_ready "$base" && return 0
  done
  return 4
}

case "${1:-}" in
  prepare)
    N="${2:?N}"; D="${3:?devices}"; RUN="${4:?run_id}"; GATE="${5:-/root/expl/$RUN.gate.json}"
    # The pool is decided by the caller, not by N. scripts/resources.mk maps N=1 to
    # a single core, which is correct for the legacy 1:1 core-per-instance default
    # but is explicitly forbidden by the W4 gate: W4 runs one replica on the same
    # two-core pool 4-5 that N=2 uses, so the workload isolates replica count and
    # not CPU. An explicit POOL_CPUSET therefore wins; only an unset one falls back
    # to the make arithmetic.
    if [[ -n "${POOL_CPUSET:-}" ]]; then
      pool="$POOL_CPUSET"
    elif (( N == 1 )); then
      pool=4                                        # as scripts/resources.mk
    else
      pool="4-$((3 + N))"
    fi
    mkdir -p "$(dirname "$GATE")"
    export POOL_CPUSET="$pool"
    make -s check N="$N" >/dev/null
    # Never reset deployment data during qualification. Shrinking device-rest to
    # one replica is required before provisioning, but databases/volumes remain.
    # Plain compose stop omits profile services. Restart the exporter after NATS
    # is ready rather than retaining a connection from the previous cold start.
    docker compose --profile export stop app-http-export
    EDGEX_REPLICAS=1 docker compose up -d
    boot_deadline=$((SECONDS + SCALE_TIMEOUT))
    while true; do
      if ! curl -sf -m3 -o /dev/null $META/ping || ! curl -sf -m3 -o /dev/null http://127.0.0.1:59986/api/v3/ping; then
        if (( SECONDS >= boot_deadline )); then
          setup_failure_dump "$GATE.failure.txt" "single-replica boot did not become pingable within ${SCALE_TIMEOUT}s"
          echo "single-replica boot did not become pingable within ${SCALE_TIMEOUT}s" >&2
          exit 4
        fi
        sleep 2
      else
        break
      fi
    done
    provision_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' edgex-device-rest-1 || true)
    if ! ensure_replica_ready edgex-device-rest-1 "http://$provision_ip:59986"; then
      setup_failure_dump "$GATE.failure.txt" "provisioning replica never exposed its ingest route ($provision_ip)"
      exit 4
    fi
    # Re-derive: a repair restart may have recreated the container on a new bridge address.
    provision_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' edgex-device-rest-1 || true)
    [[ -n "$provision_ip" ]] || {
      setup_failure_dump "$GATE.failure.txt" "provisioning replica has no bridge address"
      exit 4
    }
    # Retain the numeric runtime API evidence without private configuration
    # secrets or device payloads. These are setup observations, not estimates.
    fetch_json() { # $1=url $2=jq filter $3=output ; any failure writes the dump
      local body
      body=$(curl -fsS "$1" || true)
      if [[ -z "$body" ]] || ! jq -e . >/dev/null 2>&1 <<<"$body"; then
        setup_failure_dump "$GATE.failure.txt" "could not retrieve JSON evidence from $1"
        return 1
      fi
      jq "$2" <<<"$body" > "$3"
    }
    fetch_json "$META/device/all?limit=1" '{apiVersion,totalCount}' "$GATE.retained-inventory.json" || exit 4
    fetch_json "$META/config" '{apiVersion,Service:.config.Service}' "$GATE.metadata-discovery.json" || exit 4
    fetch_json "http://$provision_ip:59986/api/v3/config" '{apiVersion,Service:.config.Service}' "$GATE.driver-discovery.json" || exit 4
    # Read without jq -e: a parse failure must not be reported as the helper's own
    # "gate failed" exit 4. Missing/non-numeric values are setup failures here instead.
    retained=$(jq -r '.totalCount // empty' "$GATE.retained-inventory.json")
    metadata_limit=$(jq -r '.Service.MaxResultCount // empty' "$GATE.metadata-discovery.json")
    driver_limit=$(jq -r '.Service.MaxResultCount // empty' "$GATE.driver-discovery.json")
    reserve=${EDGEX_INVENTORY_RESERVE:-$D}
    if [[ ! "$retained" =~ ^[0-9]+$ || ! "$metadata_limit" =~ ^[0-9]+$ || ! "$driver_limit" =~ ^[0-9]+$ ]]; then
      setup_failure_dump "$GATE.failure.txt" "unreadable inventory/discovery evidence (retained='$retained' metadata='$metadata_limit' driver='$driver_limit')"
      echo "unreadable inventory/discovery evidence: retained='$retained' metadata='$metadata_limit' driver='$driver_limit'" >&2
      exit 4
    fi
    if (( retained + reserve > metadata_limit || retained + reserve > driver_limit )); then
      setup_failure_dump "$GATE.failure.txt" "retained inventory $retained plus reserve $reserve exceeds discovery limits $metadata_limit/$driver_limit"
      echo "retained inventory $retained plus reserve $reserve exceeds discovery limits $metadata_limit/$driver_limit" >&2
      exit 4
    fi

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
        (( try < 10 )) || {
          setup_failure_dump "$GATE.failure.txt" "device creation failed in batch at offset $off"
          echo "device creation failed in batch at $off: $(head -c 300 <<<"$res")" >&2
          exit 4
        }
        sleep 3
      done
    done

    echo "scaling to N=$N, starting the export service"
    EDGEX_REPLICAS="$N" docker compose up -d
    EDGEX_REPLICAS="$N" docker compose --profile export up -d app-http-export
    scale_deadline=$((SECONDS + SCALE_TIMEOUT))
    running_replicas() { docker ps -q --filter name=edgex-device-rest --filter status=running 2>/dev/null | wc -l; }
    while [[ $(running_replicas) != "$N" ]]; do
      if (( SECONDS >= scale_deadline )); then
        setup_failure_dump "$GATE.failure.txt" "only $(running_replicas)/$N device-rest replicas running after ${SCALE_TIMEOUT}s"
        echo "scale-out did not reach $N running device-rest replicas within ${SCALE_TIMEOUT}s" >&2
        exit 4
      fi
      sleep 2
    done

    echo "gate: every replica accepts every device"
    replicas='[]'
    for c in $(docker ps --format '{{.Names}}' --filter name=edgex-device-rest | sort); do
      ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$c" || true)
      if ! wait_http_ping "http://$ip:59986" "$c"; then
        setup_failure_dump "$GATE.failure.txt" "replica $c ($ip) never answered its HTTP ping"
        exit 4
      fi
      if ! ensure_replica_ready "$c" "http://$ip:59986"; then
        setup_failure_dump "$GATE.failure.txt" "replica $c ($ip) never exposed its ingest route after $REPAIR_RESTARTS bounded restart(s)"
        exit 4
      fi
      # A repair restart may have recreated the container on a new bridge address;
      # the device POSTs below must target the live one.
      ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$c" || true)
      [[ -n "$ip" ]] || {
        setup_failure_dump "$GATE.failure.txt" "replica $c has no bridge address"
        exit 4
      }
      for ((attempt=1; attempt<=3; attempt++)); do
        readiness_attempt=$attempt
        statuses="$GATE.$c.attempt$attempt.http-statuses"
        seq -f "bench-$RUN-%05g" 0 $((D - 1)) | xargs -P 4 -I{} curl -s -m 10 -o /dev/null -w '%{http_code}\n' \
          -X POST -H 'Content-Type: application/json' -d "{\"run_id\":\"$RUN-gate\",\"seq\":-1,\"scheduled_send_ms\":0}" \
          "http://$ip:59986/api/v3/resource/{}/reading" > "$statuses" || true
        ok=$(grep -c '^200$' "$statuses" || true)
        (( ok == D )) && break
        (( attempt == 3 )) || sleep 5
      done
      replicas=$(jq -c --arg c "$c" --argjson ok "$ok" --argjson d "$D" --argjson attempts "$readiness_attempt" '. + [{container: $c, devices_accepted: $ok, devices: $d, readiness_attempts: $attempts}]' <<<"$replicas")
      echo "  $c: $ok/$D devices accepted"
    done
    meta=$(metadata_state)
    jq -n --arg run "$RUN" --argjson n "$N" --argjson d "$D" --arg pool "$pool" --argjson r "$replicas" --argjson m "$meta" \
      --argjson repairs "$REPAIRS_USED" \
      --arg t "$(date -u +%FT%TZ)" '{run_id: $run, n: $n, devices: $d, pool_cpuset: $pool, time_utc: $t,
        helper_revision: 2, replica_repair_restarts: $repairs,
        device_rest: $r, core_metadata: $m} |
      .reasons = ([.device_rest[] | select(.devices_accepted != .devices) | "\(.container) accepted \(.devices_accepted)/\(.devices) devices"]
        + (if .core_metadata.restarts > 0 or .core_metadata.panics > 0
           then ["core-metadata restarts \(.core_metadata.restarts), panics \(.core_metadata.panics)"] else [] end)) |
      .pass = (.reasons | length == 0)' > "$GATE"
    jq -c '{pass, reasons, core_metadata}' "$GATE"
    jq -e .pass "$GATE" >/dev/null || {
      setup_failure_dump "$GATE.failure.txt" "gate failed: $(jq -c '.reasons' "$GATE")"
      echo "GATE FAILED: run is INVALID (setup failure)" >&2
      exit 4
    }
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
  *) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
