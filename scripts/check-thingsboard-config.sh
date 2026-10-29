#!/usr/bin/env bash
# Static D28 checks (no containers are started, nothing is deployed):
#   1. the vendored official files match their recorded SHA-256 and git blob IDs;
#   2. `docker compose config` renders both HEAD and the working tree;
#   3. the only differences from the pinned pre-D28 baseline are the intended D28 keys, and heap, CPU, memory, replicas, environment and
#      dependencies are identical for every service.
# Usage: scripts/check-thingsboard-config.sh        exit 0 pass, 1 check failed, 2 tooling error
set -euo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
tb="$here/deploy/thingsboard"
conf="$tb/tb-node/conf"

command -v docker >/dev/null || { echo "docker is required" >&2; exit 2; }
command -v git >/dev/null || { echo "git is required" >&2; exit 2; }

echo "== 1. vendored upstream files (v4.3.1.6)"
(cd "$conf" && sha256sum -c SHA256SUMS)
blob_ok=1
expect_blob() {  # expect_blob FILE BLOB_SHA1
  got="$(git -C "$here" hash-object "$1")"
  if [[ "$got" == "$2" ]]; then echo "ok    git blob $(basename "$1") $got"; else echo "FAIL  git blob $(basename "$1"): $got != $2"; blob_ok=0; fi
}
expect_blob "$conf/thingsboard.conf" 6e1a0a45ebeb89e4ee9c2ebdcc79d36f0fbccaee
expect_blob "$conf/logback.xml" 3642c489fc62d7312958891be7e06bda10160e07
[[ $blob_ok == 1 ]] || { echo "upstream blob check failed" >&2; exit 1; }

echo "== 2. render the pinned pre-D28 baseline and the working-tree compose"
tmp="$(mktemp -d)"
head_compose="$tb/.compose.head.yaml.tmp"
trap 'rm -rf "$tmp" "$head_compose"' EXIT
# The D28 baseline is the pre-D28 commit, not HEAD: after any commit that carries the D28 compose the
# working tree matches HEAD and the intended diff is zero, so "only D28 keys" must be checked from there.
D28_BASELINE=${D28_BASELINE:-73dea4d5493280125c7d330356a90433d78889d2}
git -C "$here" show "$D28_BASELINE:thingsboard/compose.yaml" > "$head_compose"
docker compose --project-directory "$tb" -f "$head_compose" --env-file "$tb/.env.example" config --format json > "$tmp/head.json"
docker compose --project-directory "$tb" -f "$tb/compose.yaml" --env-file "$tb/.env.example" config --format json > "$tmp/cur.json"
echo "rendered: $(wc -c < "$tmp/head.json") and $(wc -c < "$tmp/cur.json") bytes"

echo "== 3. differences from the pinned pre-D28 baseline"
python3 - "$tmp/head.json" "$tmp/cur.json" "$conf/thingsboard.conf" "$conf/logback.xml" <<'PY'
import json, sys, xml.etree.ElementTree as ET
head, cur = (json.load(open(p)) for p in sys.argv[1:3])
conf_text = open(sys.argv[3], encoding="utf-8").read()
logback = sys.argv[4]
fails, passed = [], 0
def check(ok, msg):
    global passed
    if ok:
        passed += 1
    else:
        print("FAIL  " + msg)
        fails.append(msg)

HEAP_KEYS = ("environment", "cpuset", "mem_limit", "memswap_limit", "depends_on", "deploy", "image", "restart", "logging")
TB = ("tb-core", "tb-rule-engine")
hs, cs = head["services"], cur["services"]
check(set(hs) == set(cs), f"service set differs from HEAD")
for name in sorted(hs):
    for key in HEAP_KEYS:
        check(hs[name].get(key) == cs[name].get(key), f"{name}.{key} unchanged")
    other = set(hs[name]) | set(cs[name])
    changed = sorted(k for k in other if hs[name].get(k) != cs[name].get(k))
    allowed = {"entrypoint", "volumes"} if name in TB else set()
    stray = [k for k in changed if k not in allowed]
    check(not stray, f"{name}: changed keys {changed or 'none'} only D28 keys")

for name in TB:
    ep = cs[name].get("entrypoint")
    # compose prints the escaped form ($$HOSTNAME); the container shell receives $HOSTNAME after interpolation
    unescaped = [x.replace("$$", "$") for x in ep] if ep else ep
    check(unescaped == ["/bin/sh", "-c", "export TB_SERVICE_ID=$HOSTNAME && exec start-tb-node.sh"],
          f"{name} entrypoint exports TB_SERVICE_ID from HOSTNAME and execs the official launcher")
    vols = {v.get("target"): v for v in cs[name].get("volumes", [])}
    cfg = vols.get("/config", {})
    check(cfg.get("read_only") is True and cfg.get("type") == "bind", f"{name}: /config is a read-only bind")
    check(str(cfg.get("source", "")).endswith("thingsboard/tb-node/conf") or str(cfg.get("source", "")).endswith("/tb-node/conf"),
          f"{name}: /config source is the vendored tb-node/conf directory")
    log = vols.get("/var/log/thingsboard", {})
    check(log.get("type") == "volume", f"{name}: /var/log/thingsboard is a writable named volume")

check(set(cur.get("volumes", {})) - set(head.get("volumes", {})) == {"tb-core-logs", "tb-rule-engine-logs"},
      "top-level volumes added: tb-core-logs, tb-rule-engine-logs only")

# the heap and JVM settings must stay exactly as configured; the official file must not override them
for name, env_key in (("tb-core", "JAVA_OPTS"), ("tb-rule-engine", "JAVA_OPTS")):
    check(cs[name]["environment"].get(env_key) == hs[name]["environment"].get(env_key),
          f"{name} JAVA_OPTS unchanged")
check("-Xmx" not in conf_text and "-Xms" not in conf_text, "pinned thingsboard.conf sets no heap (-Xms/-Xmx)")
check("ExitOnOutOfMemoryError" in conf_text, "pinned thingsboard.conf contains ExitOnOutOfMemoryError (verified, not yet effective)")

try:
    root = ET.parse(logback).getroot()
    files = [f.text for f in root.iter("file")]
    check(files == ["/var/log/thingsboard/${TB_SERVICE_ID}/thingsboard.log"],
          "logback file path lies under the writable /var/log/thingsboard volume and uses TB_SERVICE_ID")
except ET.ParseError as e:
    check(False, f"logback.xml parses ({e})")

print(f"{passed} checks passed")
if fails:
    print(f"{len(fails)} check(s) failed")
    sys.exit(1)
print("all static D28 checks passed")
PY
