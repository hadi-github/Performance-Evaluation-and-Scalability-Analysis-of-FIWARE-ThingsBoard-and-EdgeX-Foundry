# Contributing

## Ground rules

1. **One repository, one `main`.** Platforms are separated by *directories* (`deploy/<platform>/`) and
   configuration, never by long-lived platform branches. Short-lived feature branches merge through pull
   requests; naming: `docs/…`, `fix/…`, `feat/…`, `chore/…`, `experiment/<topic>`.
2. **Qualification integrity first.** Any change that affects measurement semantics (arrival schedule, identity
   scheme, counters, unit handling, verdict gates) needs a `fix/qualification-integrity`-style PR with:
   unit-explicit labels, unique identities, complete run records, and re-run evidence — see `docs/status.md` §2
   for the campaign-grade gate.
3. **Raw results belong outside Git.** Never commit `results/**` content (ignored; amendment files next to runs
   are the exception). Evidence held in `evidence/` is regenerated only by `scripts/evidence-manifest.py`, with
   hashes kept verifiable — never silently re-generated after a move (`evidence/README.md` records the mapping).
4. **Preregistration is immutable.** `research/preregistration/preregistration-archive/` bytes never change;
   method changes are new dated protocols under `research/protocols/`.
5. **Sites of record.** Claims live in `docs/status.md` and `docs/qualification/qualification-blockers.md`. Do not
   scatter new "current state" claims into session narratives; `docs/archive/sessions/` is append-only archival.

## Local verification (no servers needed)

```bash
go build ./... && go vet ./... && go test ./...
python3 scripts/check-preregistration.sh
bash -n scripts/*.sh deploy/*/scripts/*.sh workloads/legacy/run.sh
```

Disruptive live tests (smoke, qualification runs) run only on the explicitly pre-registered hosts and are never
part of CI defaults — see `docs/operations/server-access-preflight.md`.

## Pull requests

Use `.github/pull_request_template.md`. A PR that changes deploy/ or workloads/ paths must show
`docker compose config` renders for the affected platform(s) and the updated path map in `evidence/README.md`
when historical references are involved.
