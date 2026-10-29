# Server-access preflight (read-only)

**Recorded 2026-10-27.** Purpose: the ordered checklist to run **before and right after** receiving host access
(B1), so that the first contact with the SUT/load hosts makes no changes and produces evidence usable for the
later gates. Every command is read-only unless marked. No credentials, hostnames beyond the pre-registered
targets, or connection methods are stored here; access details are supplied by the owner out of band and are not
recorded in the repository.

Targets (pre-registered): SUT `87.107.191.107`, load `87.107.191.32`. Any deviation from these is a D1-level
finding, not an operational detail.

## Gate A — identity (do first; all read-only)

| # | Check | Command pattern | Evidence to record |
|---|---|---|---|
| A1 | Reachability and route (`ping`, `traceroute`) | `ping -c3 <host>`; `traceroute <host>` | output, loss % |
| A2 | Authenticated host identity: OS, kernel, hostname, uptime | `hostname; uname -a; cat /etc/os-release; uptime -p` | full transcript |
| A3 | Expected host? Match against the host manifest (S5 fingerprint): CPU count, memory size, disks | `lscpu 2>/dev/null | grep -E '^(CPU\(s\)|Model name)'`; `free -b`; `lsblk -d -o NAME,SIZE,MODEL` | values vs manifest |
| A4 | Clock sanity and NTP: date/UTC, chrony/ntpd status, offset | `date -u; chronyc tracking 2>/dev/null || ntpq -p 2>/dev/null` | date, offset; feeds B11 |
| A5 | Kernel versions and any pending-reboot state | `uname -r; ls /var/run/reboot-required* 2>/dev/null` | kernel string |

## Gate B — collision check (read-only; anything unexpected stops)

| # | Check | Command pattern | Stop condition |
|---|---|---|---|
| B1c | Nothing already listening on our ports | `ss -tlnp 2>/dev/null | head -40` | any listener that belongs to another workload |
| B2c | Docker state as found: `docker ps -a`, compose projects, images | `docker ps -a; docker compose ls 2>/dev/null` | unknown projects/containers running |
| B3c | Swap state / `swappiness` (fixed-resource plan assumes none or accounted) | `swapon --show; sysctl vm.swappiness` | swap active on SUT unless in manifest |
| B4c | Existing benchmark data: `results`-style directories present | `ls -la ~ /opt 2>/dev/null | head -30` | prior run data present → owner decision first; never delete |
| B5c | Disk headroom for logs + evidence | `df -h / /var` | < 20 % free ⇒ report before any run |

## Gate C — platform version pins (read-only)

- `docker images` — which tag/digest of thingsboard/orion/iot-agent/quantumleap images are **present** (may
  differ from pins; absence means a pull step will be needed).
- `docker inspect <c> --format '{{.Image}}'` on any running component to map running images to digests.
- ThingsBoard: `TB_VERSION` value from the installed `.env` (the pin is 4.3.1.6; any different value found on
  the host is a blocker-level finding).
- Orion pin: requested version in any found compose `.env` compared with the repo pin 4.5.0.

## Gate D — first mutation gate (NOT authorized in this step)

Any of the following is a change and requires explicit owner authorization after Gate C:

- `docker pull`, `make install`/`up` (creates containers/volumes), schema installs, index creation (D27),
  starting exporters, writing anything outside `/tmp`, or modifying service configuration (D28).
- Clock synchronization **changes** (ntpdate/chrony manual step) — those are mutations; offsets are recorded
  read-only in Gate A.

## What to bring back

One transcript per host: the full Gate A/B/C output, clearly labelled with command and timestamp, stored under
the run-evidence tree of the first authorized session (not in this repo's committed results without the owner's
instruction), plus a one-line summary per gate. The transcript is the input for resetting B1 in
`docs/qualification/qualification-blockers.md` and for the pre-deployment verification of D28.

## Rules

- Read-only until Gate D is authorized in writing.
- No credentials, hostnames in scripts, SSH keys or connection strings in the repository; commands above use
  placeholders only.
- Any check that cannot run (tool missing, permission denied) is recorded as such, not skipped silently.
