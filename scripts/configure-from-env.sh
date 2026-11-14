#!/usr/bin/env bash
# Validate .env and generate the SSH config every script uses.
#
#   scripts/configure-from-env.sh            # validate + write $IOTBENCH_SSH_CONFIG
#   scripts/configure-from-env.sh --check    # validate only; never writes
#   scripts/configure-from-env.sh --print    # show the resolved configuration
#
# This is the one command a new deployment runs after editing .env. It refuses to
# write a config that cannot work (missing key file, unparsable port), so a typo
# fails here instead of halfway through a 45-minute run.
set -euo pipefail
cd "$(dirname "$0")/.."

MODE=write
TRUST=0
for arg in "$@"; do
  case "$arg" in
    --check) MODE=check ;;
    --print) MODE=print ;;
    --trust) TRUST=1 ;;
    *) echo "usage: $0 [--check|--print|--trust]" >&2; exit 2 ;;
  esac
done

# Resolve every value through the single loader, so this script and the Python
# tools can never disagree about what a variable means.
eval "$(python3 - <<'PY'
import shlex, sys
sys.path.insert(0, 'scripts')
import iotbench_config as c
for k, v in (('SUT', c.SUT), ('LOAD', c.LOAD), ('SUT_IP', c.SUT_IP), ('LOAD_IP', c.LOAD_IP),
             ('SSH_USER', c.SSH_USER), ('SSH_PORT_SUT', c.SSH_PORT_SUT),
             ('SSH_PORT_LOAD', c.SSH_PORT_LOAD), ('SSH_KEY', c.SSH_KEY),
             ('JUMP', c.JUMP), ('SSH_CONFIG', c.SSH_CONFIG),
             ('SUT_ROOT', c.SUT_ROOT), ('LOAD_ROOT', c.LOAD_ROOT)):
    print(f'{k}={shlex.quote(v)}')
PY
)"

if [[ "$MODE" == print ]]; then
  python3 scripts/iotbench_config.py
  exit 0
fi

fail=0
note() { printf '  %-28s %s\n' "$1" "$2"; }
bad()  { printf '  %-28s %s\n' "$1" "$2" >&2; fail=1; }

echo "resolved configuration:"
note "SUT alias"      "$SUT"
note "SUT address"    "$SUT_IP"
note "load alias"     "$LOAD"
note "load address"   "$LOAD_IP"
note "ssh user"       "$SSH_USER"
note "ssh ports"      "sut=$SSH_PORT_SUT load=$SSH_PORT_LOAD"
note "jump host"      "${JUMP:-<direct>}"

# --- validation -------------------------------------------------------------
for pair in "IOTBENCH_SUT_IP:$SUT_IP" "IOTBENCH_LOAD_IP:$LOAD_IP"; do
  name=${pair%%:*}; val=${pair#*:}
  [[ -n "$val" ]] || bad "$name" "must not be empty"
done

# An address that is still the qualified default is legal but worth saying out
# loud: on a new network it is almost certainly a forgotten edit.
[[ "$SUT_IP"  == "87.107.191.107" ]] && note "note" "SUT_IP is still the old default"
[[ "$LOAD_IP" == "87.107.191.32"  ]] && note "note" "LOAD_IP is still the old default"

for p in "$SSH_PORT_SUT" "$SSH_PORT_LOAD"; do
  [[ "$p" =~ ^[0-9]+$ ]] && (( p > 0 && p < 65536 )) || bad "ssh port" "invalid port: $p"
done

if [[ -n "$SSH_KEY" ]]; then
  # Expand ~ exactly as ssh would.
  key="${SSH_KEY/#\~/$HOME}"
  if [[ ! -f "$key" ]]; then
    bad "IOTBENCH_SSH_KEY" "not a file: $key"
  else
    # Reject only what ssh itself rejects: any group/other permission. 600 and
    # 400 are both correct, so compare the last two octal digits against 00.
    perms="$(stat -c '%a' "$key" 2>/dev/null || echo 600)"
    if [[ "${perms: -2}" != "00" ]]; then
      bad "IOTBENCH_SSH_KEY" "too open ($perms); chmod 600 $key"
    else
      note "ssh key" "$key ($perms)"
    fi
  fi
fi

(( fail == 0 )) || { echo "configuration is invalid; nothing written" >&2; exit 1; }
[[ "$MODE" == check ]] && { echo "configuration OK"; exit 0; }

# --- trust new host keys ----------------------------------------------------
# The launcher pins `StrictHostKeyChecking=yes`, which is deliberate: a benchmark
# must not silently accept a changed host key mid-campaign. The cost is that a
# brand-new host has no key yet, so trust it explicitly, once, here.
#
# This is not "disable checking": the keys are scanned and recorded, and every
# later connection verifies against them. Re-run only when a host is genuinely
# rebuilt, and prefer comparing the fingerprint out of band first.
if (( TRUST )); then
  known="$HOME/.ssh/known_hosts"
  touch "$known"
  chmod 600 "$known"
  for pair in "${LOAD}:${LOAD_IP}:${SSH_PORT_LOAD}" "${SUT}:${SUT_IP}:${SSH_PORT_SUT}"; do
    alias=${pair%%:*}; rest=${pair#*:}; ip=${rest%%:*}; port=${rest##*:}
    echo "  host keys for ${alias} (${ip}:${port}):"
    if ! scan="$(timeout 20 ssh-keyscan -T 10 -p "$port" "$ip" 2>/dev/null)" || [[ -z "$scan" ]]; then
      # A host that is down cannot be trusted yet, and that must not block the
      # other one: the config is still worth writing, and StrictHostKeyChecking
      # will refuse the untrusted host at connection time regardless.
      note "not trusted" "${alias}: unreachable, re-run --trust when it is up"
      continue
    fi
    printf '%s\n' "$scan" | ssh-keygen -lf - 2>/dev/null | sed 's/^/    /'
    # Drop any previous entry for this address so a rebuilt host is not refused
    # forever, then record the scanned keys under the name ssh will look up.
    ssh-keygen -R "$ip" -f "$known" >/dev/null 2>&1 || true
    if [[ "$port" != "22" ]]; then ssh-keygen -R "[$ip]:$port" -f "$known" >/dev/null 2>&1 || true; fi
    printf '%s\n' "$scan" | sed "s|^${ip}|${alias}|" >> "$known"
    note "trusted" "$alias -> $ip:$port"
  done
fi

# --- generate the ssh config ------------------------------------------------
[[ -n "$SSH_CONFIG" ]] || { echo "IOTBENCH_SSH_CONFIG is empty; nothing to write" >&2; exit 1; }
out="${SSH_CONFIG/#\~/$HOME}"
mkdir -p "$(dirname "$out")"
tmp="$(mktemp "${out}.XXXXXX")"
{
  echo "# GENERATED by scripts/configure-from-env.sh from .env — do not edit by hand."
  echo "# Regenerate after changing .env; it is the only place these values live."
  echo
  echo "Host ${LOAD}"
  echo "    HostName ${LOAD_IP}"
  echo "    User ${SSH_USER}"
  echo "    Port ${SSH_PORT_LOAD}"
  [[ -n "$SSH_KEY" ]] && echo "    IdentityFile ${SSH_KEY/#\~/$HOME}"
  echo "    IdentitiesOnly yes"
  echo "    StrictHostKeyChecking accept-new"
  echo "    ServerAliveInterval 15"
  echo "    ServerAliveCountMax 3"
  if [[ -n "$JUMP" ]]; then
    # The jump host is itself an ssh alias or user@host; if it carries a port it
    # must be spelled ProxyJump user@host:port.
    echo "    ProxyJump ${JUMP}"
  fi
  echo
  echo "Host ${SUT}"
  echo "    HostName ${SUT_IP}"
  echo "    User ${SSH_USER}"
  echo "    Port ${SSH_PORT_SUT}"
  [[ -n "$SSH_KEY" ]] && echo "    IdentityFile ${SSH_KEY/#\~/$HOME}"
  echo "    IdentitiesOnly yes"
  echo "    StrictHostKeyChecking accept-new"
  echo "    ServerAliveInterval 15"
  echo "    ServerAliveCountMax 3"
} > "$tmp"
chmod 600 "$tmp"
mv "$tmp" "$out"
echo "wrote $out"

# Prove it: an ssh config that parses but cannot reach the host is worse than
# none, so report reachability without failing (a host may legitimately be down).
for host in "$LOAD" "$SUT"; do
  if timeout 20 ssh -F "$out" -o BatchMode=yes -o ConnectTimeout=8 "$host" 'true' 2>/dev/null; then
    note "reachable" "$host"
  else
    note "UNREACHABLE" "$host (config is written; fix the host or the network)"
  fi
done

echo
echo "next: export IOT_BENCH_SSH_CONFIG=$out   # or set IOTBENCH_SSH_CONFIG in .env"
