# Workloads (k6): a fixed request rate per platform

Three k6 workloads, one per platform, all using the same **open workload model**. They send `RATE` telemetry
requests per second for `DURATION` (default 10 min, after a 1 min warm-up), however fast or slowly the platform
answers. This is the difference from the reference JMeter plans, which are closed loops (`threads` × `loops`): there
the rate is whatever the platform sustains, so slow platforms get less load.

They run on the load-generator host (ramonavm1) in Docker and stream their metrics to the local Prometheus
(`../monitoring`).

```bash
./run.sh fiware 200                 # 200 req/s for 10 min (+1 min warm-up), 100 devices
./run.sh edgex 500 5m --devices 1000
./run.sh thingsboard 100 10m --warmup 2m --n 1
```

| Option | Default | Meaning |
|---|---|---|
| `<rate>` | (required) | target arrival rate, requests/s |
| `[duration]` | `10m` | length of the measured (steady) phase |
| `--warmup` | `1m` | warm-up at the same rate before it, excluded from the results (`0s` = none) |
| `--devices` | `100` | devices provisioned in setup; requests go to them round-robin |
| `--n` | `1` | number of platform instances deployed on the SUT (only recorded, for tagging) |
| `--sut` | `87.107.191.107` | SUT address (the platform's HAProxy) |
| `--max-vus` | `2 × rate` (min 50) | concurrency cap. An arrival-rate executor needs about rate × latency VUs |
| `--keep` | off | do not delete the devices afterwards |
| `--no-verify` | off | skip the stored-message check |

Environment: `K6_IMAGE` (default `docker.arvancloud.ir/grafana/k6:1.8.1`), `K6_CPUSET` (default `0-2`; core 3
belongs to Prometheus/Grafana), `PROM_URL`, `GRAFANA_URL`.

## What each workload does

Every iteration is one HTTP request carrying one device's integer sensor values (same ranges as the JMeter plans:
temperature 15-45, humidity 30-80, pressure 950-1150).

| | setup (not measured) | each request | verification after the run |
|---|---|---|---|
| `thingsboard.js` | tenant login, create the devices, read each one's access token | `POST :8080/api/v1/<token>/telemetry` `{"temperature","humidity","pressure"}` | stored points per device (`agg=COUNT`) must equal successful sends |
| `fiware.js` | service group (API key) + batch device provisioning on the IoT Agent | `POST :7896/iot/d?k=<key>&i=<device>` `t\|..\|h\|..\|p\|..` | every device's entity updated during the run (Orion keeps only the latest value, so no exact count) |
| `edgex.js` | device profile `bench-workload` + batch devices, then wait until device-rest accepts readings | `POST :59986/api/v3/resource/<device>/temperature` `<int>` | stored events per device in core-data must equal successful sends |

Afterwards the devices (and, for FIWARE, entities and the service group) are deleted. EdgeX events stay in core-data.
Run `make clean` on the SUT between benchmark cells.

**The compared unit is the request.** device-rest accepts one resource per request, so an EdgeX request carries one
reading, while ThingsBoard and FIWARE requests carry three values.

ThingsBoard stamps telemetry on arrival, and two messages of one device in the same millisecond would overwrite each
other. A device is hit every `devices/rate` seconds, so keep `rate/devices` well below 1000 (e.g. ≤ 200).

## Results

`results/<testid>/` (testid = `<platform>-n<N>-r<rate>-<UTC time>`):

- `summary.json`: verdict, steady-phase requests / achieved rate / error rate / latency percentiles, verification,
  plus the SUT health check (`manifest`);
- `summary.txt`: the k6 end-of-test summary and the verdict;
- `k6-summary.json`: k6's full summary data;
- `manifest.json`: parameters, start/end time, k6 image, OOM kills / container starts on the SUT during the run,
  replicas per service.

All k6 metrics are also in Prometheus, labelled `testid`, `platform`, `n`, `rate` and `scenario` (`warmup`/`steady`).
Each run is also marked as a Grafana annotation.

A run is **INVALID** (the load or the SUT was not what the cell claims) if:
- k6 dropped iterations: it ran out of VUs, so the target rate was not delivered (raise `--max-vus`, or the SUT is
  saturated beyond what the generator can compensate for);
- the steady-phase achieved rate differs from the target by more than 1%;
- a SUT container was OOM-killed or (re)started during the run.

Errors (non-200 responses) and stored ≠ sent are **findings**, not invalidity: they are what the platform did at that
load, and they are reported.

## Files

```
run.sh              one benchmark cell (docker run grafana/k6 ... + SUT health check + Grafana annotation)
lib/common.js       options/scenarios, device selection, sensor values, summary and verdict
lib/k6-summary.js   local copy of jslib.k6.io/k6-summary 0.1.0 (the VMs cannot reach jslib.k6.io)
thingsboard.js fiware.js edgex.js
```
