# Bounded recovery check — 2026-11-01

Continued from `9f907b4` with a clean working tree. The load host's actual observation timestamps
were **2026-10-31 23:03 UTC**; this note retains those times rather than assuming a timezone/date.

One SUT SSH check through the existing `ramonavm2` alias timed out. No further SUT SSH retries
were made. Direct load-host access succeeded: identity `ramona-vm-q`, uptime 17 days, low load.
One load-host request to SUT receiver port 18083 also timed out after five seconds.
Its existing Prometheus reports every SUT target down (timeouts or no route to host).
This establishes continued reachability failure, **not its cause** or the SUT's current identity/state.

Evidence: `results/recovery-20261101/`, with checksums and a dated amendment index. The raw
Prometheus target response and SHA-256 hashes of all six preserved load-host manifests are retained.

| Claim | Current state |
|---|---|
| Load-host identity, monitoring, Grafana, yaade and original ceiling sink | Observed running; untouched |
| SUT identity, processes, containers, listeners, OOM/restart/network logs | Unavailable while SSH is inaccessible |
| Receiver ledger completeness, shutdown/flush and immediate-run reconciliation | Unavailable; no empty substitute or inferred loss |
| Service restoration and new qualification runs | Not attempted |
| Common 200/s ceiling and campaign readiness | Not qualified / main campaign NO-GO |

No receiver was stopped/recreated, no output path reused, no firewall/network settings changed,
and no volumes/devices touched. The SUT's **last confirmed** service actions remain those in
`qualification-next-20261031.md`: benchmark stacks stopped for isolation, four exporters retained,
two controlled Python receivers started; their current state is unknown. On the load host, all
six interrupted/finished run manifests remain present and no k6 container appeared in the inventory.

**Exact next action:** restore the existing SSH access path through infrastructure diagnosis;
then follow the recovery order in `qualification-next-20261031.md` before any test or restoration:
capture current SUT identity/system/network evidence, recover receiver files and verify process
identities before graceful flush, preserve checksums and `original-running.txt`, and reconcile
the immediate run if its ledger is complete. Do not invoke the unpatched remote EdgeX prepare
script, restart old truncating receivers, reset data, or infer a cause without evidence.
