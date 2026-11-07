# Publication and repository handoff

## What is ready, and what is not

The repository contains deployment/tooling source, offline tests, measurement
contracts and frozen controls for the bounded ordinary campaign. It is **not a
completed experiment dataset or final paper artifact**. The current verified
counts and unresolved attempt are recorded in [status](../status.md); historical
results and failures retain their original interpretation.

Remaining scientific work is official execution/recovery and evidence-based
reporting under the [operator runbook](ordinary-campaign.md). Some publication
decisions are also outstanding; do not describe them as completed experiments
or silently close them because the source has been committed.

## Owner publication decisions

- **Licence:** no root `LICENSE` has been adopted. `CITATION.cff` explicitly says
  `UNLICENSED`; making source visible does not choose a reuse licence. The owner
  must select the intended licence and confirm rights before adding licence text.
- **Citation:** author identity, public repository URL and release metadata in
  `CITATION.cff` are placeholders. Set them to the real owner/project values
  before publishing a citable release; no author or DOI is invented here.
- **Evidence archive:** choose the public archive location, access/privacy
  policy and stable identifier. Review/redact bundles before upload and publish
  input/output checksums. A clone alone cannot reproduce raw-data claims.
- **History/security:** review the full Git history with a dedicated secret
  scanner before making the existing history public. A current-tree check is
  not full-history clearance. Never push first and rely on later deletion to
  remove a secret. No history rewrite, force-push or new public remote is part
  of this handoff.

## Source versus evidence

The root `.gitignore` excludes environment secrets, generated executables and raw
`results/**`, with a narrow exception for frozen campaign control/source files
and historical amendments. See the [control-directory policy](../../results/feasible-campaign-20261106/README.md).
Do not use `git add -f results` or commit run ledgers, logs, database exports,
private review receipts, keys, `.env`, compiled binaries or mutable lock/state.

The documents include deployment-specific IPs, public demo-tenant defaults and
paths for reproducibility. Those are **not production credential recommendations**.
Keep private credentials and SSH keys out of command lines, manifests and Git.
Before publication, review whether deployment identifiers should be redacted;
do not change frozen evidence bytes in place to do so.

## Verification before handoff

Run from the repository root; these commands do not launch benchmark traffic:

```sh
go build ./cmd/...
go vet ./internal/... ./cmd/... ./tests/...
go test -short -count=1 ./internal/... ./cmd/... ./tests/...
go test -race -count=1 ./internal/runner ./cmd/benchctl
python3 -B -m unittest discover -s tests -p '*_test.py'
python3 -B -m unittest discover -s tests/offline -p 'test_*.py'
node --test tests/offline/*.test.mjs
bash scripts/check-preregistration.sh
git diff --check
```

Scoped Go package lists avoid compiling archived copies of source in a local
evidence tree. Omit `-short` to run Docker integration when Docker is available;
these tests may pull a pinned k6 image and generate traffic only to local fixtures.
An offline pass is not live qualification. See the [test catalogue](../development/testing.md).
CI also renders Compose descriptors and
checks shell/Python syntax. Missing raw evidence checks are **skipped**, never
reported as successful checksum verification merely because plans exist.

For a real raw bundle, run its retained checksums and exact offline reproduction
commands separately. Do not regenerate historical checksum manifests to hide a
mismatch. Chart dependencies are optional; do not substitute plots for primary
per-message evidence.

## Git workflow boundary

The independent source/documentation handoff re-review returned **ACCEPTED**
after its initial conditional findings were corrected. This is not public-release
or runtime-continuation clearance. The corrections include portable shell
temporary directories, explicit fail-closed historical qualification dependency,
no fixed-sleep ordinary readiness, current SSH viewing guidance and accurate CI
fixture labels. The progress observer additionally rejects missing/null identity
fields. Pathologically blocked observer I/O can still delay finalization; it is
not a hard nonblocking guarantee. Licence, citation, dedicated full-history
secret scanning and archive arrangements remain owner publication gates.

Review the staged diff and file list, then create local commits. This handoff
does not itself push to GitHub, create a release, pick a licence, change remote
visibility or rewrite history. Check `git status` before starting independent
GitHub work, and preserve ignored local evidence separately from the repository.
After official execution, update status, review result tables and archive links,
then tag a reproducible release only when its evidence and metadata are complete.
