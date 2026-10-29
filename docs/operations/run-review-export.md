# Run review dashboard and exports

Additive patch delivered as `iot-monitoring-delivery.tar.gz`, integrated into this repository:
`monitoring/grafana/dashboards/iot-run-review.json` (provisioned) and `scripts/export-run.py`.

## Dashboard

Open UID `iot-run-review` in Grafana. Select **one run** and an **absolute** time window.
Keep the existing detailed dashboard (`iot-benchmark-overview`). No live deployment was performed
as part of the delivery.

## Export

Requires Python 3 and `matplotlib` (install in a virtual environment). Run on the monitoring host
with access to the local Prometheus; no public port needed. Replace the example with actual
recorded UTC measurement boundaries and run ID:

```bash
python3 scripts/export-run.py --run-id ACTUAL_RUN_ID --platform thingsboard \
  --start 2026-10-29T10:00:00Z --end 2026-10-29T10:10:00Z \
  --prometheus http://127.0.0.1:9090 --grafana http://YOUR_MONITORING_HOST:3000 \
  --step 5 --context 60 --out results/ACTUAL_RUN_ID/visuals
```

- Output directory must not already exist. Use `--dashboard` if the dashboard JSON moves.
- Export **immediately after the run**, allowing two scrape intervals for ingestion.
- Start/end are measurement boundaries; 60 seconds before/after provide context.
- Export the full observation window separately if drain extends further. Keep monitoring active
  through drain/reconciliation. **Never infer time bounds from run names.**

## Outputs

CSV with UTC/labels/phase, raw query-response JSON, `export.json` with exact expressions/units/
query status and a fixed Grafana URL, 12 PNG/SVG pairs, offline `index.html`, `SHA256SUMS`.

## Interpretation cautions

- Rates use a 30-second lookback, so boundary samples include earlier activity.
- Prometheus evaluation samples are **not** per-message raw records.
- Do not calculate run p99 by averaging chart percentiles.
- Exact CPU cost requires cgroup endpoint counters.
- Infrastructure is time-scoped, not run-labelled; isolate workloads.
- `NO_DATA` stays missing, never zero. Query errors exit 2 and preserve partial results.
- Exit 0 is export success, **not** campaign qualification.
- Kafka no-data is expected outside ThingsBoard.
- Only the intended `benchctl remote-write` path supplies labelled workload metrics; legacy runs
  may be empty.

## Live acceptance (pending)

1. Verify provisioning and live label/query coverage (done for integration; live unverified).
2. Run a bounded benchctl smoke with remote write and authoritative
   warmup/measurement/drain timestamps.
3. Export its exact window and compare with Prometheus and raw run records; verify missing
   metrics display NO DATA.
4. Retrieve original 200/s summaries; resolve the `1.469761` vs `1469.8 ms` discrepancy without
   editing historical evidence.
5. Complete identity/schedule/record-completeness qualification and repeat 2x headroom through
   the authoritative runner. Dashboard health does not establish PASS.
6. Report readiness per claim. Preserve volumes/devices/yaade; do not start the campaign before
   existing gates pass.

## Validation

Synthetic Prometheus fixture tested API retrieval, missing series, sample gaps, 12 PNG/SVG pairs,
CSV/HTML and invalid run-id rejection. Fixture data are not experimental results. Live Grafana
rendering and server deployment remain unverified.
