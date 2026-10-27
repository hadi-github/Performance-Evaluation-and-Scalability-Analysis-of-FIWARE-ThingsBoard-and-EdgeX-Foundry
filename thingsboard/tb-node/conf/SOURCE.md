# Pinned ThingsBoard tb-node configuration (D28)

Vendored for `thingsboard/compose.yaml` (`tb-core`, `tb-rule-engine`), mounted read-only at `/config`.

| File | Upstream path (tag `v4.3.1.6`) | Git blob SHA-1 (upstream) | SHA-256 (vendored bytes) |
|---|---|---|---|
| `thingsboard.conf` | `docker/tb-node/conf/thingsboard.conf` | `6e1a0a45ebeb89e4ee9c2ebdcc79d36f0fbccaee` | `ee04dfcd1c4ca130ad8d7f5e5221b2c18c779a58d5aaf619041e4167fd1465b2` |
| `logback.xml` | `docker/tb-node/conf/logback.xml` | `3642c489fc62d7312958891be7e06bda10160e07` | `5a45db03ac118f12bfdd0d8fe769d27aba5a905b44e6d0274da67951ddd66dcb` |

Retrieval (2026-10-26): exact bytes fetched from `https://raw.githubusercontent.com/thingsboard/thingsboard/v4.3.1.6/...`.
The blob SHA-1 values match the GitHub contents API (`contents/docker/tb-node/conf?ref=v4.3.1.6`) and `git hash-object`
on the vendored files. Repository: `https://github.com/thingsboard/thingsboard`.

## Launcher (reference, not vendored)

The image's launcher `msa/tb-node/docker/start-tb-node.sh` (tag `v4.3.1.6`, blob `eb363be7efd7a4ad5770648f8857144885aa050d`,
SHA-256 `30a83f08f51dd222e0c46d22beab3d682432af4351fe3befea6e5c94adee0d39`):

- If `/config` exists it sets `LOGGING_CONFIG=/config/logback.xml` and sources `/config/thingsboard.conf`.
- Otherwise it uses the image's own `/usr/share/thingsboard/conf` copies.

## Differences from the image's built-in logback (recorded deviation)

The image copies `msa/tb-node/docker/logback.xml` (blob `2b2c01020f2c06dae1d285e1bd5fb73008e307fd`, 908 bytes,
stdout only). The pinned official file used here (`docker/tb-node/conf/logback.xml`) adds file appenders under
`/var/log/thingsboard/${TB_SERVICE_ID}/`. Mounting `/config` therefore changes logging from stdout-only to file logging.
That is why the compose change adds a writable log volume. Recorded as the D28 addendum in `docs/design-decisions.md`.

## What the pinned `thingsboard.conf` sets

It appends to `JAVA_OPTS` and does not set `-Xms`/`-Xmx`, so the compose heap values are unchanged. It adds:
GC log and heap dump under `/var/log/thingsboard/${TB_SERVICE_ID}…`, `-XX:+IgnoreUnrecognizedVMOptions`,
`-XX:+HeapDumpOnOutOfMemoryError`, G1 options, and `-XX:+ExitOnOutOfMemoryError`.

## Verification

```bash
cd thingsboard/tb-node/conf && sha256sum -c SHA256SUMS
```

`scripts/check-thingsboard-config.sh` checks the hashes and the compose wiring statically. Effective JVM flags and
log behaviour are **not** verified until a running deployment is inspected (see `docs/thingsboard-d28-verification.md`).
