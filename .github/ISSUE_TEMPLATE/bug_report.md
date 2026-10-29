# Bug report: <one line>

## What happened vs what was expected

## Environment

- Branch/commit:
- Hosts involved: (none / load host / SUT — refer only to pre-registered hosts)
- Platform + N:

## Evidence

Attach or reference run outputs (`results/<testid>/summary.json`, `manifest.json`,
`evidence/evidence-integrity-manifest.csv` rows). Do not paste raw `results/**` binaries.

## Checks already done

- [ ] `go build ./... && go vet ./... && go test ./...` passes
- [ ] Relevant `docker compose config` renders
- [ ] `sha256sum -c evidence/evidence-integrity.sha256` still clean (unchanged evidence)
