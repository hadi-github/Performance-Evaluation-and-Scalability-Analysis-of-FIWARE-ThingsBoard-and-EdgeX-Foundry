#!/usr/bin/env bash
# Deploy the generator (committed code + static Go binaries) to the load-generator host.
#
#   scripts/deploy-generator.sh [root@host] [--jump root@ramonavm2] [--allow-dirty]   (SSH_KEY=path: identity to use)
#
# Ships `git archive HEAD` (never the working tree) plus bin/{benchctl,fixture-target} and GIT_REV ("<sha> <dirty>"),
# so every manifest written there names the exact commit. Refuses a dirty tree unless --allow-dirty (then GIT_REV
# says dirty=true, although the shipped files are still exactly HEAD). Untracked files on the host (.env,
# results/) are kept.
set -euo pipefail
cd "$(dirname "$0")/.."
HOST=root@87.107.191.32 JUMP="" ALLOW=0
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
git archive --format=tar HEAD -o "$tmp/repo.tar"
echo "$(git rev-parse HEAD) $dirty" > "$tmp/GIT_REV"
tar czf "$tmp/bundle.tgz" -C "$tmp" bin repo.tar GIT_REV
ssh_opts=(-o BatchMode=yes); [[ -n "$JUMP" ]] && ssh_opts+=(-J "$JUMP")
[[ -n "${SSH_KEY:-}" ]] && ssh_opts+=(-o IdentitiesOnly=yes -i "$SSH_KEY")
scp -q "${ssh_opts[@]}" "$tmp/bundle.tgz" "$HOST:/tmp/iot-bench-generator.tgz"
ssh "${ssh_opts[@]}" "$HOST" 'set -e; D=/root/iot-bench-deploy; mkdir -p $D/bin; t=$(mktemp -d)
  tar xzf /tmp/iot-bench-generator.tgz -C $t; rm /tmp/iot-bench-generator.tgz
  tar xf $t/repo.tar -C $D; cp $t/bin/* $D/bin/; cp $t/GIT_REV $D/GIT_REV; rm -rf $t; echo "deployed $(cat $D/GIT_REV) to $(hostname):$D"'
