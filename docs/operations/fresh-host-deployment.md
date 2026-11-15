# Fresh-host deployment

Standing the benchmark up on a **new pair of hosts** (SUT + load generator), from a
clean checkout to a verified, measurable state. This is the redeploy runbook for
migrating to a rented VPS or replacing a host.

[Server-access preflight](server-access-preflight.md) covers the complementary job:
*checking* a host you have just been given. Read that first if the host is
unfamiliar; read this page when you intend to build the stack.

**Nothing on this page authorizes an experimental run.** It ends at a verified
deployment. Launching a workload still needs the applicable qualification plan, an
owner-authorization receipt and `execution_authorized`. Deploying is not permission
to measure.

## 0. What you need before starting

| Requirement | Detail |
|---|---|
| Two hosts | SUT and load generator, kept separate. Never colocate the generator with a platform. |
| SUT sizing | 8 vCPU / 16 GB is the qualified shape. `scripts/resources.mk` refuses an `N` whose pool exceeds the host's cores; `scripts/check-resources.sh` refuses a stack whose limits exceed memory minus a 1.5 GB reserve. A smaller host fails closed, which is the intended behaviour. |
| SUT OS baseline | Linux, Docker Engine with Compose v2, `jq`, `curl`, `git`, `make`. Cores `0–3` reserved for host/LB/infra (see [resource layout](../../README.md#resource-layout-fixed-pinned)). |
| Load-host baseline | Docker (for the sink and monitoring stack), `jq`, `curl`, Go toolchain **only if** you build there (prebuilt binaries are the normal path). |
| Installing the baseline | `scripts/bootstrap-host.sh load` or `scripts/bootstrap-host.sh sut` installs those packages over SSH and prints the resulting host state. It refuses to go further than packages: it does not change the kernel, the Docker version the distribution ships, or any sysctl, because those are measurement inputs. `--dry-run` reports what is missing. |
| Network | SUT reachable from the load host on the platform ingress ports; load host reachable from the SUT for outbound delivery. No platform port exposed to the public internet except through the firewall you set up in step 6. |
| Out-of-band | SSH access details and credentials. **Never commit them.** `IOT_BENCH_SSH_CONFIG` points at a private SSH config; host keys are trusted explicitly, never `StrictHostKeyChecking=no`. |

Ports the stack publishes, per platform:

| Platform | Telemetry ingress | Admin/API | HAProxy stats |
|---|---|---|---|
| ThingsBoard | `8080` (HTTP device API `/api/v1`) | `8080` (REST + UI) | `8404` |
| FIWARE | `7896` (UltraLight HTTP) | `1026` (Orion), `4041` (IoT Agent) | `8404` |
| EdgeX | `59986` (device-rest) | `59880` (core-data), `59881` (core-metadata) | `8404` |

Every one of these is overridable in the platform `.env`. If you change a port,
record it: run manifests name the target they used.

## 1. Configure the deployment: one file

Every host, address, port and path the tooling uses comes from **one file**,
`.env` at the repository root. Nothing else needs editing when the testbed
moves: not the scripts, not the campaign plans, not this page.

```sh
cp .env.example .env
$EDITOR .env                 # the two addresses are the only required edits
scripts/configure-from-env.sh --trust
```

`configure-from-env.sh` validates the file, writes the SSH config that every
script shares, and — with `--trust` — scans and records the host keys. Read its
output: it prints the resolved configuration and the fingerprints it recorded.

| Variable | Default (qualified testbed) | Meaning |
|---|---|---|
| `IOTBENCH_SUT_HOST` | `ramonavm2` | SSH alias of the system under test |
| `IOTBENCH_LOAD_HOST` | `ramonavm1` | SSH alias of the load generator |
| `IOTBENCH_SUT_IP` | `87.107.191.107` | the address the generator connects to |
| `IOTBENCH_LOAD_IP` | `87.107.191.32` | the address platforms push receipts to |
| `IOTBENCH_SSH_USER` | `root` | SSH user on both hosts |
| `IOTBENCH_SSH_PORT_SUT` / `_LOAD` | `22` | SSH ports (they often differ: an ISP NAT may map one) |
| `IOTBENCH_SSH_KEY` | *(empty)* | identity file; a leading `~` is expanded |
| `IOTBENCH_JUMP` | *(empty)* | `user@host` when the load host is only reachable through the SUT |
| `IOTBENCH_SSH_CONFIG` | `.iotbench-ssh-config` | generated config, used by every ssh/scp call |
| `IOTBENCH_SUT_ROOT` / `_LOAD_ROOT` | `/srv/iot-bench/{sut,load}` in `.env.example`; `/root/iot-qual-*` in code | deployment roots on each host |
| `IOTBENCH_TB_COMPOSE` | `/root/iot-qual-tb-20261105/deploy/thingsboard/compose.yaml` | ThingsBoard compose file |
| `IOTBENCH_FW_DIR` | `/root/iot-bench-deploy-cand-auto20261027` | FIWARE root; `edgex/` is expected beneath it |
| `IOTBENCH_REMOTE_RESULTS` | `/root/iot-qual-load-20261105/…` | generator results root on the load host |
| `IOTBENCH_SINK_PORT` | `18091` | capture port for the sink |
| `IOTBENCH_POOL_CPUSET` / `_INFRA_CPUSET` / `_REPLICAS` | `4-5` / `2-3` / `2` | CPU budget and replica count |

**The code defaults are the qualified testbed, byte for byte.** With no `.env`
at all, the tooling behaves exactly as it did for the published runs — a missing
file must never silently point a run somewhere new. `.env.example` deliberately
differs in one respect: it names the date-free `/srv/iot-bench/{sut,load}`
layout, which is what a fresh deployment should use. The code defaults keep the
dated `/root/iot-qual-*-20261105` paths because live campaign state depends on
them.

### A reachable host is not a configured host

Two things must both be true, and a ping only proves the first:

```sh
ssh -F .iotbench-ssh-config ramonavm1 'hostname; test -d /run && echo RUN_OK'
```

The command must *run* and print `RUN_OK`, not merely open a connection. See
[A reachable port is not a usable host](troubleshooting.md#a-reachable-port-is-not-a-usable-host).

### Moving the testbed later

Changing `IOTBENCH_SUT_IP` is enough: the campaign plans are hash-bound, so they
are never rewritten. When the configured address differs from a plan's
`target`, the launcher writes a **target-only overlay**, proves with a
full-field comparison that nothing else changed, and stores both hashes and
both hosts as `plan-overlay.json` in that run's evidence. Only the host may
differ — a moved port or a changed device count still fails closed.

To inspect an overlay offline:

```sh
scripts/rebase-plan.py workloads/campaigns/<batch>/plans/<plan>.json /tmp/plan.json
scripts/rebase-plan.py workloads/campaigns/<batch>/plans/<plan>.json /tmp/plan.json --verify
```

A new host with a different identity is a **D1-level finding**, not an
operational detail — see [server-access preflight](server-access-preflight.md)
Gate A. Record the change in the qualification record before any run whose
evidence will be compared with earlier cells. Comparability across hosts is a
reviewed claim, not an assumption.

## 2. Choose the replica setting deliberately

`N` is the number of platform instances and is the workload axis of W4. It also
sets the CPU pool:

| `N` | `POOL_CPUSET` | Cores |
|---|---|---|
| 1 | `4` | 1 |
| 2 | `4-5` | 2 |
| 4 | `4-7` | 4 |

> **The frozen B0 and W1–W3 configurations use N=2, pool `4-5`, k=2.** That is the
> shape to reproduce if the purpose is to continue those batches on the new host.
>
> Be careful with N=1: the shared Makefile arithmetic maps it to a **single core**
> (`4`). The W4 qualification gate explicitly excludes that — it requires one replica
> *on the same two-core pool 4–5*. Deploying at N=1 through `make up N=1` therefore
> does **not** produce the W4 configuration. Do not treat a default N=1 deployment as
> W4 evidence.

## 3. Deploy the system under test

Each platform has its own directory and its own `.env`. Only one platform is ever up
at a time.

```sh
cd deploy/<platform>
cp .env.example .env        # make also does this; edit only where the host differs
```

Then, per platform:

**ThingsBoard** — needs a one-shot schema install the first time.

```sh
cd deploy/thingsboard
make install                # postgres + valkey, then tb-install if the schema is absent
make up N=2                 # renders config, runs check-resources.sh, starts the stack
make smoke N=2              # requires TB_LOAD_DEMO=true at install time
```

Three JVMs boot on the same pool cores, so allow up to ~15 minutes.

**FIWARE**

```sh
cd deploy/fiware
make pull                   # first time only; pinned tags must match .env
make up N=2
make smoke N=2
```

**EdgeX** — images are **built**, not pulled: the official images lack NATS, and NATS
queue groups are EdgeX's only competing-consumer mechanism. The build needs
`github.com` and a Go module proxy; where the proxy is blocked, set `GO_BUILDER_BASE`
to a local builder image that configures `GOPROXY`.

```sh
cd deploy/edgex
make build
make up N=2
make smoke N=2
```

For the bounded inventory cohort, EdgeX additionally uses the discovery override
`deploy/edgex/compose.feasible-discovery.yaml` (`ALL_SERVICES_SERVICE_MAXRESULTCOUNT`,
16,384). The launcher applies it; a hand-started stack needs `-f` with that file if
it must discover the full cohort.

**Capacity beyond 16,384 devices requires an opt-in amendment.** The frozen 16,384
cap admits only one W4 EdgeX repetition. Raising it is not sufficient on its own:
device-rest also has to stop fetching the shared profile once per device, which
binds at the readiness gate *before* the inventory check. Enabling it is a recorded
departure from the frozen configuration and needs its own qualification receipt:

```sh
# build the amended image once on the SUT (needs github.com; set GO_BUILDER_BASE
# to a builder image that configures GOPROXY where proxy.golang.org is blocked)
./deploy/edgex/scripts/build-patched-device-rest.sh        # prints the image tag

# select it for a run; anything but exactly "1" leaves the frozen path active
export IOTBENCH_EDGEX_DISCOVERY_AMENDMENT=1
```

The build fails closed if the dedup patch is not actually in effect, which a plain
exit code would not catch — upstream builds with `-mod=readonly`, which silently
ignores `vendor/`. See
[the amendment record](../../research/findings/edgex-profile-fetch-dedup-amendment-20261115.md).

### What `make smoke` proves, and what it does not

It proves each replica is alive behind HAProxy and that messages are stored exactly
once (ThingsBoard, EdgeX). FIWARE's smoke covers reachability and HTTP ordering only,
because Orion keeps just the latest value; exactly-once there needs the notification
sink. **A passing smoke test is not campaign readiness** — the real gates are the
bounded backend, initialized-ingest-route and all-device replica gates in the
[operator runbook](ordinary-campaign.md). Do not measure because a ping succeeded.

## 4. Deploy the load generator

The generator host ships `git archive HEAD` plus prebuilt static binaries, so the
manifest names the exact commit and Go need not be installed remotely.

```sh
# from the repository root, on the workstation; hosts come from .env
scripts/deploy-generator.sh
```

The host comes from `IOTBENCH_SSH_USER`, `IOTBENCH_LOAD_HOST` and
`IOTBENCH_SSH_CONFIG`/`IOTBENCH_SSH_KEY`, so no argument is needed once `.env` is
right. An explicit `user@host` argument still overrides it. The destination is
`IOTBENCH_LOAD_ROOT`, defaulting to the qualified `/root/iot-bench-deploy`.

Notes:

- It **refuses a dirty working tree**. That guard is deliberate: a run whose evidence
  cannot name its source commit is not citable. `--allow-dirty` exists for
  deployment experiments only and stamps `GIT_REV` with `dirty=true`.
- It ships `benchctl`, `fixture-target` and `sink`. Those are the measurement
  binaries; building them elsewhere and copying them in by hand loses the
  provenance link.
- Untracked files already on the host (`.env`, `results/`) are preserved.
- **It ships to `/root/iot-bench-deploy` (or `IOTBENCH_LOAD_ROOT`), which is not
  where the campaign launcher reads from.** The two paths serve different purposes
  and both are needed:
  - `deploy-generator.sh` installs a **usable generator checkout** on the load host
    (binaries plus a source tree you can run by hand).
  - The launcher **stages its own source per run** into `IOTBENCH_REMOTE_RESULTS/source/`
    and verifies the staged manifest before executing. It does not read
    `/root/iot-bench-deploy`.

  The binaries in that staged tree are built **on the workstation, not the load
  host**: the launcher runs `go build` for `benchctl` and `sink` from the
  repository, unless `IOT_BENCH_PREBUILT_DIR` points at a directory holding
  `benchctl`, `sink` and `build-provenance.json`. That prebuilt path is verified
  against the controller's Go source hash, so a stale or mismatched binary is
  refused rather than silently used.

  So a fresh load host needs no Go toolchain and no manual binary placement for a
  campaign run; what it needs is that `IOTBENCH_REMOTE_RESULTS` exists and is
  writable. The **workstation** running the launcher does need Go when
  `IOT_BENCH_PREBUILT_DIR` is unset. Using a stale `/root/iot-bench-deploy` tree
  from an earlier experiment is harmless for the launcher, but do not mistake it
  for the binaries a run uses.

### Prove the generator works before there is a SUT

The load host can be verified on its own, which is what to do while waiting for a
SUT to exist. `benchctl` runs k6 in a container against a local fixture endpoint,
so nothing outside the host is involved:

```sh
ssh -F .iotbench-ssh-config ramonavm1 '
  cd /root/iot-bench-deploy && mkdir -p /tmp/gen-check &&
  nohup ./bin/fixture-target -addr 127.0.0.1:18080 >/tmp/gen-check/f.log 2>&1 &
  sleep 2 &&
  ./bin/benchctl run -plan workloads/plans/fixture-smoke.json \
    -results /tmp/gen-check -run-id gen-check -cpuset 0-1 -env GOGC=400'
```

A working host prints `== gen-check: VALID` with `achieved_ratio=1` on all three
phases and exits non-zero on anything less. This exercises the pinned k6 image,
the Docker path, the CPU pinning and the plan-resolution code — everything a real
run uses except the platform. **It is a pilot only** (`pilot_only: true`): it is
not a measurement and must never be aggregated.


## 5. Configure outbound delivery

The sink is how delivery is observed independently of the HTTP acknowledgement. It
must be reachable from the SUT, and it must be configured through the platform's own
delivery mechanism — not by reading the platform's database.

```sh
source scripts/iotbench-env.sh    # addresses come from .env
scripts/outbound.sh thingsboard   # uses $SUT_IP and $SINK_URL_BASE
scripts/outbound.sh fiware
# edgex: no action — the export profile in compose.yaml carries the sink URL
```

Both addresses come from `.env`; `SINK_URL` still overrides. On the load host the
sink URL must name an address the SUT can reach, which is why `IOTBENCH_LOAD_IP`
is a separate variable from the load host's SSH alias.

Two sink ports appear in the controls and they are not interchangeable:
`18090` is the general sink used by `scripts/sink.sh` and `scripts/outbound.sh`;
**`18091` is the dedicated capture port** the reviewed campaign launcher uses for
measurement. The launcher refuses to start if `18091` is already occupied, and
deliberately does not treat `18090` as a free-port candidate because it is expected
to be in use. Point a qualification at the capture port the plan expects, not at
18090.

Reverse it with `scripts/outbound.sh thingsboard-remove` for runs that need the
observer-overhead-free path.

> **FIWARE payload caveat.** The frozen sink and history subscriptions do **not**
> forward a custom `padding` attribute, so a stored-payload claim cannot be made from
> them. This is a known, documented limitation of the W3 evidence path — see the
> internal record. Do not "fix" it silently for one platform; it changes comparability
> and needs its own reviewed change.

## 6. Monitoring and firewall

```sh
scripts/monitoring.sh up                    # load host: Prometheus, Grafana, node-exporter, cAdvisor
scripts/monitoring.sh exporters-install     # SUT, once: firewall + container-info timer
scripts/monitoring.sh exporters-up <platform>
scripts/monitoring.sh check                 # targets must be UP for the running platform
```

The SUT firewall restricts exporter ports to the load host and is re-applied after
every Docker start by `iot-bench-firewall.service`. Re-run `exporters-install` on a
new SUT; it is not covered by `deploy/`.

A target being DOWN is expected for the two platforms that are not currently running.
It is not expected for the running one. With no SUT at all, every SUT target is DOWN;
`scripts/monitoring.sh check` distinguishes that case (it reports the stack is down and
exits 3) from a running stack, so a bare refusal is not mistaken for a target fault.

> **Record the host's OS and CPU model in the deployment notes.** The
> pre-registration pins the load host's *size* (4 cores / 8 GB), not its OS
> version or CPU model. Those still matter for comparability: publish the two
> values you actually deployed on, so a reader comparing cells knows whether the
> generator host changed underneath them.

## 7. Verify before you call it deployed

Deployment is complete when all of the following hold, and you have the output saved
as evidence:

1. **Identity and clock** — Gate A of the [preflight](server-access-preflight.md)
   re-run on the new host: kernel, CPU, memory, disks, NTP offset. A large clock
   offset invalidates the primary latency endpoint.
2. **Rendered resources match the configuration you claim** — `make check` passed, and
   the effective cpuset/memory/replica values for every service were captured, not
   just the intended ones.
3. **Every replica is UP behind HAProxy** — exactly N servers per replicated tier, one
   exporter instance, HAProxy present at every N.
4. **Ingest works on every replica** — not just one. EdgeX in particular has a history
   of a second `device-rest` replica failing to register its ingest route.
5. **Sink delivery arrives** — send a small number of messages and confirm the sink
   receives them with the expected identity.
6. **No unexpected listener or container** on either host — anything belonging to
   another workload stops the deployment (preflight Gate B).
7. **Evidence boundary is understood** — raw evidence is external to Git. A clone
   alone does not reproduce earlier results; see the
   [campaign control README](../../results/feasible-campaign-20261106/README.md).
8. **Monitoring renders and scrapes** — `scripts/render-prometheus.py --check` is
   current, `promtool` accepts `monitoring/prometheus.rendered.yml`, and Prometheus
   reports the load-host targets `up`. With no SUT running, every SUT-exporter
   target is expected to be `down`; confirming that shape is the check, because
   the alternative is silently scraping the literal string `${SUT_IP}`.

   ```sh
   scripts/render-prometheus.py --check
   scripts/monitoring.sh check      # promtool + live target table
   ```

## 8. What must not be done during a redeploy

These are campaign-integrity rules, not style preferences.

- **Never `clean`, `down -v`, `docker volume rm` or reset data** to "start fresh" on a
  host that holds existing cohorts or history. Preserved state is part of the
  experiment; deleting it loses evidence that cannot be regenerated.
- **Never reuse a consumed run ID.** A failed attempt keeps its ID and its failure.
  A successor gets a new ID and a new receipt.
- **Never run two platforms at once**, and never run the load generator and a
  qualification concurrently. `isolated_one_platform_at_a_time` is a control, not a
  preference.
- **Never edit the frozen scope, matrix or plans** to accommodate a new host. Host
  changes are recorded as findings; the plans are frozen.
- **Never commit credentials, private keys, SSH configs or host secrets.** The
  preflight rules on this are absolute.
- **Never claim capacity from a deployment check.** A stack that starts is not a
  qualified configuration.

## 9. After deployment

A verified deployment is the prerequisite for, not a substitute for, the next
qualification. Go to the [operator runbook](ordinary-campaign.md) and
[persistent execution](persistent-execution.md), and confirm against
[current status](../status.md) which workload and platform is actually next.
