# ThingsBoard D28 — implementation, verification status and rollback

**Recorded 2026-10-26 (Prompt 5).** Authorization: D28 Text + Impl/checks. **No deployment, container restart or OOM
experiment was performed.** The OOM issue is **not** claimed resolved.

Evidence labels: **[V]** verified locally from the vendored files or the rendered Compose configuration;
**[U]** read from the pinned upstream source (tag `v4.3.1.6`); **[G]** not verified until a running deployment is
inspected.

## 1. What changed (local files only)

| File | Change |
|---|---|
| `thingsboard/tb-node/conf/thingsboard.conf` | new: pinned official file (`docker/tb-node/conf/thingsboard.conf`) [U] |
| `thingsboard/tb-node/conf/logback.xml` | new: pinned official file (`docker/tb-node/conf/logback.xml`) [U] |
| `thingsboard/tb-node/conf/SHA256SUMS`, `SOURCE.md` | hashes, blob IDs, retrieval record, deviations |
| `thingsboard/compose.yaml` | `tb-core` and `tb-rule-engine`: `entrypoint`, `/config:ro` mount, writable `/var/log/thingsboard` named volume; two top-level volumes |
| `scripts/check-thingsboard-config.sh` | static checks (hashes, blob IDs, rendered HEAD-vs-working-tree comparison) |

Nothing else in `compose.yaml` changed. Heap settings (`TB_CORE_JAVA_OPTS`, `TB_RULE_ENGINE_JAVA_OPTS`), cpusets,
memory limits, `memswap_limit`, replica counts (`TB_REPLICAS`), environment, dependencies, image and restart policy are
identical to HEAD. The check script confirms this for every service.

## 2. Why the entrypoint is overridden (deviation)

- The official `docker-compose.yml` at `v4.3.1.6` sets `TB_SERVICE_ID` per service (`tb-core1`, …) [U]. Our compose runs
  replicas from one service, so a fixed name would be shared by every replica.
- The pinned `logback.xml` writes to `/var/log/thingsboard/${TB_SERVICE_ID}/thingsboard.log`, and the pinned
  `thingsboard.conf` writes the GC log and heap dump to `/var/log/thingsboard/${TB_SERVICE_ID}-…` [U].
- If `TB_SERVICE_ID` is unset, logback leaves `${TB_SERVICE_ID}` literal and all replicas of a service would share one
  path [V, reasoning from the file contents; not observed].
- The compose header says the platform already derives the service id from the container hostname when the variable is
  empty [V, `thingsboard/compose.yaml` comment]. The override exports that same hostname, so the platform identity is
  intended to be unchanged.
- The image's `CMD` is `start-tb-node.sh` [U, `msa/tb-node/docker/Dockerfile`]. The override keeps that script and only
  exports the variable first.

Recorded deviation: **the container's entrypoint is `/bin/sh -c 'export TB_SERVICE_ID=$HOSTNAME && exec start-tb-node.sh'`**.
Compose prints the escaped form `$$HOSTNAME`; the container shell receives `$HOSTNAME` (Compose escaping rule) [U-spec, G-runtime].

## 3. Behaviour changes that are expected (not memory changes)

| Item | Before (HEAD, no `/config`) | After |
|---|---|---|
| Logback | image's stdout-only `logback.xml` (908 bytes) [U] | official file logging plus STDOUT (`/var/log/thingsboard/<hostname>/thingsboard.log`) |
| GC log | not written by the image's own config [G] | `/var/log/thingsboard/<hostname>-gc.log`, rotated at 10 files × 10 MiB |
| Heap dump on OOM | not configured [G] | `/var/log/thingsboard/<hostname>-heapdump.bin`, up to the heap size (512 MiB per core/rule-engine replica) |
| Exit on OOM | not configured by the image's own configuration [G] | `-XX:+ExitOnOutOfMemoryError` (from the pinned `thingsboard.conf`) |
| Disk | none on the volume | `totalSizeCap` 3 GB per service for logs, plus heap dump and GC logs |

`ExitOnOutOfMemoryError` means a JVM OOM terminates the process and Docker's restart policy restarts the container. That
changes how an OOM appears (a restart, not a hung JVM). It does not make the system survive an OOM. D28 forbids claiming
OOM recovery was validated. **Not validated.**

## 4. Static verification (done, 2026-10-26)

```bash
scripts/check-thingsboard-config.sh
```

Result: vendored SHA-256 and git blob IDs match; HEAD and working-tree Compose both render; **95 checks passed**, covering:

- only `entrypoint` and `volumes` differ from HEAD, and only on `tb-core` and `tb-rule-engine`;
- `/config` is a read-only bind of `thingsboard/tb-node/conf`; `/var/log/thingsboard` is a named volume;
- the entrypoint unescapes to the intended command;
- the pinned `thingsboard.conf` does not set `-Xms` or `-Xmx`, so the compose heap values remain the only heap source;
- the pinned `logback.xml` parses and its file path lies under the volume and uses `TB_SERVICE_ID`.

These checks are **static**. They do not prove the container starts, that the volume is writable by the image's
service user, or that the effective JVM flags are as listed. Those are the live checks in §5.

## 5. Post-deployment verification (NOT executed; needs a Deploy authorization)

Run only after a separately authorized restart of `tb-core` and `tb-rule-engine`. Container names follow the Compose
project (`tb-tb-core-1`, …); adjust to the actual names from `docker compose ps`.

1. Mounted files match the vendored hashes:
   `docker exec <c> sha256sum /config/thingsboard.conf /config/logback.xml` → must equal `tb-node/conf/SHA256SUMS`.
2. Entry point and identity: `docker inspect --format '{{json .Config.Entrypoint}}' <c>`; `docker exec <c> sh -c 'echo $TB_SERVICE_ID'`
   must equal the container hostname.
3. Effective JVM flags: `docker exec <c> sh -c 'tr "\0" "\n" < /proc/1/cmdline'`. Expect `-Xms512m -Xmx512m` (or the
   configured values), `-XX:+ExitOnOutOfMemoryError`, `-XX:+HeapDumpOnOutOfMemoryError`, and `-Dlogging.config=/config/logback.xml`.
   If `jcmd` is present in the image, `jcmd 1 VM.flags` is the stronger check.
4. Writable log path: `docker exec <c> sh -c 'ls -ld /var/log/thingsboard /var/log/thingsboard/$TB_SERVICE_ID && test -w /var/log/thingsboard/$TB_SERVICE_ID && echo writable'`.
   If the directory is root-owned, logging fails silently or degrades. [G: whether the image's `/var/log/thingsboard` is owned by the service user and copied into a new named volume is not verified.]
5. Logs are written: `docker exec <c> sh -c 'ls -l /var/log/thingsboard/$TB_SERVICE_ID'`, and the STDOUT stream shows no appender errors (`docker logs <c> | grep -i 'logback\|ERROR' | head`).
6. Container state: `docker inspect --format '{{.State.OOMKilled}} {{.RestartCount}}' <c>`; record both before and after the smoke.
7. Replicas do not share a log file: at `TB_REPLICAS≥2`, each replica directory name differs.

Any mismatch stops the deployment and is recorded; the configuration is rolled back (§6).

## 6. Rollback

**Local source (no deployment):** the file was clean at HEAD, so
`git checkout -- thingsboard/compose.yaml` restores it exactly. Then remove the vendored and check files that D28 added:
`rm -r thingsboard/tb-node scripts/check-thingsboard-config.sh docs/qualification/thingsboard-d28-verification.md`.
Do not use `git clean` or `git reset`, which could remove other pre-existing uncommitted work.

**If deployed (requires a separate Deploy authorization; not executed):**

1. Restore the HEAD compose file as above.
2. `docker compose up -d --no-deps --force-recreate tb-core tb-rule-engine` (recreates both services from the restored file).
3. Keep the `tb-core-logs` and `tb-rule-engine-logs` volumes. They contain evidence. Remove them only after the logs
   are archived and hashed, and only under a separate authorization.
