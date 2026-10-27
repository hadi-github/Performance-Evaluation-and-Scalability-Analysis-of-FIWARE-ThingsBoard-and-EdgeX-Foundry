# Autonomous session handoff (auto-20261027, close-out 2026-10-27 ~12:15Z)

## What ran where (SUT = ramona-vm-2 87.107.191.107, load = ramona-vm-q 87.107.191.32)

### Left running (intentionally, with consent of the session;s own authorization; data preserved)
1. **ThingsBoard k=2 benchmark stack** — `/root/iot-bench-deploy-cand-auto20261027/thingsboard`
   (project `tb`, containers tb-tb-core-1/2, rule-engine-1/2, transports, haproxy/kafka/postgres/valkey/zookeeper).
   Registry now holds 2,013 devices (1,000 F1 `f1-auto20261027-*` + 1,000 F2 `f2-auto20261027-*` + 13 session
   probe devices) — PRESERVED, all prior benchmark volumes reused, none deleted.
   Stop/rollback: `cd …/thingsboard && docker compose down --remove-orphans` (volumes keep all data).
2. **FIWARE history candidate** — `/root/iot-bench-deploy-cand-auto20261027/fiware-history-candidate`
   (project `fiwhc`: orion :11026, quantumleap :8668, timescale, mongo fresh volumes `fwc-*`).
   Stop/rollback: same pattern (`docker compose down`, volumes kept).
3. **FIWARE baseline stack (for the D27 audit)** — `/root/iot-bench-deploy-cand-auto20261027/fiware-baseline`
   (project `fiware`, ports remapped to 11027/14041/17896/18404 so the pre-existing `fiware` ports never collide;
   uses the PRE-EXISTING `mongo-data` volume — no data touched).
4. **SUT exporters** — `/root/iot-bench-deploy-cand-auto20261027/monitoring/exporters` (node-exporter/cadvisor/
   postgres-exporter/nats-exporter on cpuset 0, host-port 9100) installed for B9/B13; safe to stop
   (`make exporters-down`) after any run watching them.

### Load host (ramonavm1 / ramona-vm-q)
- Pre-existing monitoring stack (prometheus/cadvisor/grafana/node-exporter) + `yaade` — untouched.
- Candidate generator dir **`/root/iot-bench-cand-auto20261027/workloads`** with results of the two VALID smokes
  (`thingsboard-n1-r2-20261027T114603Z`, `edgex-n1-r2-20261027T115425Z`) incl. `prom-raw/` and
  `resources-summary.json` — new session dirs, nothing overwritten.

## Evidence map (local repo `results/auto-20261027/`)
| Path | Content |
|---|---|
| `baseline-tracked.diff`, `baseline-untracked.txt` | exact pre-session dirty state |
| `preflight-sut-gateA.txt`, `preflight-load-gateA.txt` | Gates A–C transcripts (sanitized) |
| `f1/` | F1 monitors (2 CSVs), strict analyzer outputs, cgroup peaks |
| `f2/` | F2 monitors (full + idle), interval CSV (39 fragments, 0 qualifying) |
| `b9/` | EdgeX bounded smoke run files + raw Prom responses + resources-summary + offline re-check note |
| `clock/` | read-only SSH-pair offset window-1 (load→SUT), script + output |
| `d28/`, `fw/` | (created during D28/FIWARE verification; see README inside each) |

## Author decisions still required
1. F1's k=1 stop-condition → record a deviation (raise TB memory) or accept the stage's infeasibility.
2. B10: choose the max planned rate (or authorize a labelled ceiling exploration) to define 2×.
3. B4: adopt the FIWARE Blue candidate allocations into the fixed supporting tier budget.
4. B11: the cross-host clock threshold.

## Verify / resume commands (safe by default — none deploy or push)
```bash
# repo checks
go build ./... && go vet ./... && go test ./...
scripts/check-thingsboard-config.sh               # 95 static D28 checks
sha256sum -c docs/evidence-integrity.sha256       # historical evidence manifest (repo root)
cd results/auto-20261027/f1 && sha256sum -c SHA256SUMS   # F1 evidence inventory
python3 - <<'PY'  # strict interval analysis over the retained F1 CSVs
import subprocess; subprocess.run(['python3','/tmp/opencode/auto/f1-analysis.py',
 'results/auto-20261027/f1/f1-monitor1-raw.csv','results/auto-20261027/f1/f1-monitor2-raw.csv',
 '--window-out','/tmp/opencode/auto/f1-artifacts-check'], capture_output=True)
PY
# FIWARE candidate live facts (on the SUT)
ssh ramonavm2 'cd /root/iot-bench-deploy-cand-auto20261027/fiware-history-candidate && docker compose ps; docker exec fiwhc-timescale-1 psql -U postgres -d quantumleap -At -c "SET search_path TO mtopeniot; SELECT count(*) FROM etdevice;"'
# history export on the SUT
ssh ramonavm2 'FWHC_CONTAINER=fiwhc-timescale-1 bash /root/iot-bench-deploy-cand-auto20261027/scripts/history-ids.sh fiware replay-auto20261027'
# bounded smokes on the load host (each run creates a NEW results dir; never overwrites)
ssh ramonavm1 'cd /root/iot-bench-cand-auto20261027/workloads && ./run.sh thingsboard 2 1m --devices 100 --warmup 30s'
```

## Known limitations of this session's evidence
- The EdgeX core_data column audit (B14 EdgeX branch) did NOT run (no EdgeX describe command on a live stack).
- The full §4 clock procedure (before AND after a run, ≥30 pairs / ≥10 min) remains incomplete; claims unqualified.
- No OOM/kill probe on any retained deployment (out of authorization scope).
- B10 ceiling/B12/B13 remain open (rate choice needed).
