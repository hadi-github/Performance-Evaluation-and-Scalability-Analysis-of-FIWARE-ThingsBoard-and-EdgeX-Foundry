# Security policy

##_scope

This project is a research benchmark harness for self-hosted IoT platform deployments (FIWARE, EdgeX,
ThingsBoard) plus its own load-generation tooling. It is published as *tooling and exploratory qualification* —
not as a completed benchmark campaign (`docs/status.md`).

## Reporting

Report vulnerabilities privately: email the maintainers listed in `CITATION.cff` (fallback: open a GitHub
security advisory). Do not open a public issue for credential-capable findings.

## What is and is not a vulnerability in scope

- In scope: anything in this repository that ships, overwrites, or executes code on other hosts — especially
  shell scripts (`scripts/`, `deploy/*/scripts/`, `workloads/legacy/run.sh`) that `ssh`/`docker` into the
  benchmark hosts; the Yaade workflow runner (`yaade/`); image-building paths in `deploy/edgex/Makefile`.
- Out of scope: the benchmark targets themselves (FIWARE/EdgeX/ThingsBoard upstream projects), and anything that
  presupposes access the operator granted you anyway. Report those upstream.

## Secrets handling

- Only sanitized `*.env.example` templates are committed; real `.env` files are ignored (`.gitignore`).
- If you find a credential in Git history, report it; publication policy is squash-orphan `main` — see
  `docs/status.md` §5 before any push.
- Benchmark hosts are explicitly pre-registered (`docs/operations/server-access-preflight.md`); no script in this
  repo should ever create new host identities or change SSH configuration.
