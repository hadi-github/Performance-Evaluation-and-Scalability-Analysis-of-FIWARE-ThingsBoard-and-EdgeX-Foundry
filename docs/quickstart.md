# Quickstart for a new engineer

This page is a navigation aid. It does not authorize a deployment, a workload, a
history export or a publication.

## What this repository is

The project compares HTTP telemetry processing in FIWARE, ThingsBoard and EdgeX
under a fixed, recorded resource budget. The load and measurement host is kept
separate from the system under test (SUT). The benchmark distinguishes:

- application dispatch from physical wire transmission;
- HTTP acknowledgement from external sink delivery;
- sink delivery from independently retrieved history.

Read [system design](architecture.md) and [measurement and interpretation](methodology.md)
for the complete explanation.

## First 20 minutes

1. Read the [documentation index](README.md) and the
   [current status](status.md).
2. Read [platform architecture](architecture.md) and the
   [measurement guide](methodology.md).
3. Run the offline checks below. They do not start benchmark traffic.
4. Read the [ordinary-campaign runbook](operations/ordinary-campaign.md) and
   [persistent execution](operations/persistent-execution.md) before touching a
   controller.
5. Inspect the current status and evidence paths before trusting any summary.

## Offline checks

From the repository root:

```sh
go build ./cmd/...
go test -short -count=1 ./internal/... ./cmd/... ./tests/...
python3 -B -m unittest discover -s tests -p '*_test.py'
python3 -B -m unittest discover -s tests/offline -p 'test_*.py'
node --test tests/offline/*.test.mjs
bash scripts/check-preregistration.sh
```

These commands may need the declared Go, Python and Node toolchains. They are
local/offline checks; they do not deploy a platform or qualify a workload.

## Where to go next

| Question | Read |
|---|---|
| What do the terms mean? | [Glossary](glossary.md) |
| How does the system work? | [System design](architecture.md) |
| What is measured and what is not? | [Measurement guide](methodology.md) |
| What is the research framing? | [Research guide](research-guide.md) |
| How do I run or recover a campaign? | [Ordinary-campaign runbook](operations/ordinary-campaign.md) |
| How does detached execution work? | [Persistent execution](operations/persistent-execution.md) |
| What failed and what should I inspect? | [Troubleshooting](operations/troubleshooting.md) |
| How do I build and test tools? | [Test catalogue](development/testing.md) |
| How do evidence and checksums fit together? | [Evidence index](../evidence/README.md) |
| What is currently completed? | [Current status](status.md) |

## Safety rules

- A detached controller may already be running. Check [current status](status.md)
  and the live controller state before issuing any command that could deploy or
  generate traffic.
- Do not use `make clean`, `down -v`, data resets or image replacement to
  “help” a campaign.
- Do not retry a failed attempt under the same ID.
- Do not resume an old controller state after a pause or reboot.
- Do not treat a missing progress file as success.
- Do not publish credentials, private SSH configuration or raw evidence.

For proposals, qualification packets, frozen protocols and paper material, use
the existing research, methodology, qualification and archive pages. They are
preserved intentionally and are not onboarding substitutes.
