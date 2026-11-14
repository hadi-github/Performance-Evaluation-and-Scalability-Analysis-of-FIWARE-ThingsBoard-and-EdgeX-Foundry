#!/usr/bin/env bash
# Deploy the generator (committed code + static Go binaries) to the load-generator host.
#
#   scripts/deploy-generator.sh [root@host] [--jump root@ramonavm2] [--allow-dirty]   (SSH_KEY=path: identity to use)
#
# Ships `git archive HEAD` (never the working tree) plus bin/{benchctl,fixture-target,sink} and GIT_REV ("<sha> <dirty>"),
# so every manifest written there names the exact commit. Refuses a dirty tree unless --allow-dirty (then GIT_REV
# says dirty=true, although the shipped files are still exactly HEAD). Untracked files on the host (.env,
# results/) are kept.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=scripts/iotbench-env.sh
source scripts/iotbench-env.sh
HOST="${SSH_USER}@${LOAD_IP}" JUMP="" ALLOW=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --jump) JUMP="$2"; shift 2 ;;
    --allow-dirty) ALLOW=1; shift ;;
    *) HOST="$1"; shift ;;
  esac
done
dirty=false; [[ -n "$(git status --porcelain)" ]] && dirty=true
if [[ $dirty == true && $ALLOW == 0 ]]; then echo "working tree is dirty: commit first (or --allow-dirty)" >&2; exit 2; fi
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"
CGO_ENABLED=0 go build -trimpath -o "$tmp/bin/benchctl" ./cmd/benchctl
CGO_ENABLED=0 go build -trimpath -o "$tmp/bin/fixture-target" ./cmd/fixture-target
CGO_ENABLED=0 go build -trimpath -o "$tmp/bin/sink" ./cmd/sink
git archive --format=tar HEAD -o "$tmp/repo.tar"
echo "$(git rev-parse HEAD) $dirty" > "$tmp/GIT_REV"
tar czf "$tmp/bundle.tgz" -C "$tmp" bin repo.tar GIT_REV
# The target directory is configurable so a fresh deployment is not forced into
# the qualified host's path. Default is the qualified one.
REMOTE_DIR="${IOTBENCH_LOAD_ROOT:-/root/iot-bench-deploy}"
ssh_opts=(-o BatchMode=yes); [[ -n "$JUMP" ]] && ssh_opts+=(-J "$JUMP")
# `.env` may name a named config; using it keeps host-key handling identical to
# every other path (the alternative is a second, quietly different policy).
[[ -n "${SSH_CONFIG:-}" && -f "${SSH_CONFIG/#\~/$HOME}" ]] && ssh_opts+=(-F "${SSH_CONFIG/#\~/$HOME}")
[[ -n "${SSH_KEY:-}" ]] && ssh_opts+=(-o IdentitiesOnly=yes -i "${SSH_KEY/#\~/$HOME}")
scp -q "${ssh_opts[@]}" "$tmp/bundle.tgz" "$HOST:/tmp/iot-bench-generator.tgz"
ssh "${ssh_opts[@]}" "$HOST" "set -e; D='$REMOTE_DIR'; mkdir -p \$D/bin; t=\$(mktemp -d)
  tar xzf /tmp/iot-bench-generator.tgz -C \$t; rm /tmp/iot-bench-generator.tgz
  tar xf \$t/repo.tar -C \$D; cp \$t/bin/* \$D/bin/; cp \$t/GIT_REV \$D/GIT_REV; rm -rf \$t; echo \"deployed \$(cat \$D/GIT_REV) to \$(hostname):\$D\""
