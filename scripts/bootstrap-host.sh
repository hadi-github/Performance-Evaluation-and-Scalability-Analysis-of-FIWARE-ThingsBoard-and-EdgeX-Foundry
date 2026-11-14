#!/usr/bin/env bash
# Install what a benchmark host needs, idempotently.
#
#   scripts/bootstrap-host.sh load    # generator host: Docker, jq, curl, git, python3
#   scripts/bootstrap-host.sh sut     # platform host: the above plus make
#   scripts/bootstrap-host.sh load --dry-run
#
# Reads the host from `.env` (see scripts/iotbench-env.sh), so the same command
# works for any deployment. Re-running is safe: package installation is skipped
# when everything is already present.
#
# What this deliberately does NOT do: change the kernel, install a different
# Docker version than the distribution ships, tune sysctl, or touch the CPU
# governor. Those are measurement inputs, and changing them silently would make
# this host incomparable with earlier cells. If a host needs something beyond
# packages, the runbook says to record it.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=scripts/iotbench-env.sh
source scripts/iotbench-env.sh

ROLE="${1:-}"
case "$ROLE" in
  load|sut) ;;
  *) sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
DRY_RUN=0
for arg in "${@:2}"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

if [[ "$ROLE" == load ]]; then
  HOST="${SSH_USER}@${LOAD_ALIAS}"
  ALIAS="$LOAD_ALIAS"
  PACKAGES=(docker.io docker-compose-v2 jq curl git python3 ca-certificates)
else
  HOST="${SSH_USER}@${SUT_ALIAS}"
  ALIAS="$SUT_ALIAS"
  PACKAGES=(docker.io docker-compose-v2 jq curl git make python3 ca-certificates)
fi

ssh_opts=(-o BatchMode=yes -o ConnectTimeout=10)
if [[ -n "${SSH_CONFIG:-}" ]]; then
  cfg="${SSH_CONFIG/#\~/$HOME}"
  [[ -f "$cfg" ]] || { echo "IOTBENCH_SSH_CONFIG is not a file: $cfg" >&2; exit 2; }
  ssh_opts+=(-F "$cfg")
fi
[[ -n "${SSH_KEY:-}" ]] && ssh_opts+=(-o IdentitiesOnly=yes -i "${SSH_KEY/#\~/$HOME}")

echo "bootstrapping ${ROLE} host ${ALIAS} (${HOST})"
echo "  packages: ${PACKAGES[*]}"

# Prove the host is usable before installing anything: opening a session is not
# the same as running a command (see docs/operations/troubleshooting.md).
if ! probe="$(timeout 30 ssh "${ssh_opts[@]}" "$HOST" 'hostname; test -d /run && echo RUN_OK' 2>&1)"; then
  echo "  cannot run a command on ${ALIAS}:" >&2
  printf '%s\n' "$probe" | sed 's/^/    /' >&2
  exit 1
fi
printf '%s\n' "$probe" | sed 's/^/  /'

missing=()
for pkg in "${PACKAGES[@]}"; do
  if ! timeout 20 ssh "${ssh_opts[@]}" "$HOST" "dpkg -s ${pkg} >/dev/null 2>&1"; then
    missing+=("$pkg")
  fi
done
if (( ${#missing[@]} == 0 )); then
  echo "  all packages present; nothing to install"
else
  echo "  missing: ${missing[*]}"
  if (( DRY_RUN )); then
    echo "  --dry-run: not installing"
  else
    timeout 900 ssh "${ssh_opts[@]}" "$HOST" \
      "set -e; export DEBIAN_FRONTEND=noninteractive; apt-get update -qq; apt-get install -y -qq ${missing[*]}"
  fi
fi

# Report the state that matters for the benchmark, without changing it.
echo "  host state:"
timeout 60 ssh "${ssh_opts[@]}" "$HOST" '
  . /etc/os-release
  printf "    os       %s (%s)\n" "$PRETTY_NAME" "$(uname -r)"
  printf "    cpu      %s vCPU (%s)\n" "$(nproc)" "$(lscpu | sed -n "s/^Model name: *//p" | head -1)"
  printf "    memory   %.2f GiB\n" "$(free -b | awk "/Mem:/{print \$2/1073741824}")"
  printf "    docker   %s\n" "$(docker --version 2>/dev/null || echo MISSING)"
  printf "    compose  %s\n" "$(docker compose version --short 2>/dev/null || echo MISSING)"
' 2>&1 | sed 's/^/  /'

echo "  done. Re-run scripts/configure-from-env.sh --check to confirm the deployment."
