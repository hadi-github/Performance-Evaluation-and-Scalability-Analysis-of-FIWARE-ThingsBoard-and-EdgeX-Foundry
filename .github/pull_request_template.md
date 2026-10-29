# PR: <one line>

## Type

- [ ] docs (status/blockers/qualifications)
- [ ] chore (paths/CI/packaging) — no behaviour change
- [ ] fix (qualification-integrity: identity/schedule/units/gates)
- [ ] feat (new tooling capability)
- [ ] experiment (temporary, documented settings)

## What changes

## Evidence for a behaviour change

- [ ] `go build ./... && go vet ./... && go test ./...`
- [ ] affected `docker compose config` renders (platform: )
- [ ] `bash -n` on changed shell scripts
- [ ] measurement semantics touched → unit labels, unique identities, complete run records verified on the
      campaign-grade path; legacy/verdict impact explained (`docs/status.md` §2)
- [ ] evidence moved → hashes unchanged and mapping updated in `evidence/README.md`; preregistration untouched
- [ ] `sha256sum -c evidence/evidence-integrity.sha256` still verifies

## Live-server impact

None by default (CI/sandbox only) — disruptive runs require pre-registered hosts and manual dispatch.
