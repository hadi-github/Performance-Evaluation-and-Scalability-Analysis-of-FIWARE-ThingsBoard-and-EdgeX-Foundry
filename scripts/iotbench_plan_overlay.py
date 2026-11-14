"""Re-point a frozen plan at a different host without changing the workload.

The campaign plans are hash-bound: their bytes are recorded in `run-matrix.json`
and in every authorization receipt. They also embed the SUT address inside
`target`. When the testbed moves to new addresses, something has to give.

This module resolves that honestly. It produces a copy whose **`target` block
alone** points at the configured host, and it proves the workload is untouched
by comparing every other JSON path. The frozen original is never modified, and
the caller records the overlay (original hash, overlay hash, old host, new host)
next to the run so a reviewer can check that the only difference was an address.

Why not edit the plans? Because the hash is the evidence that the experiment did
not change. Editing in place would silently invalidate it. A separate, recorded
overlay keeps "the workload is exactly the frozen one" checkable.

Why not leave the address alone? Because the address is not a treatment. The
pre-registered variables are device count, rate, replica count, payload, seed and
duration; the host address is where the testbed happens to live. Changing it does
not change what is measured, and the sink URL and subscription URLs are already
environment-driven in the launcher.
"""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
from urllib.parse import urlsplit

import iotbench_config as cfg


def sha256_file(path: Path) -> str:
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def target_hosts(plan: dict) -> set[str]:
    """Every host named inside the plan's `target` block."""
    hosts = set()
    for value in (plan.get('target') or {}).values():
        if isinstance(value, str) and '://' in value:
            host = urlsplit(value).hostname
            if host:
                hosts.add(host)
    return hosts


def needs_overlay(plan: dict, host: str) -> bool:
    """True when the plan still points somewhere other than `host`."""
    hosts = target_hosts(plan)
    return bool(hosts) and hosts != {host}


def rewrite_target(plan: dict, host: str) -> dict:
    """Return a copy of `plan` with only `target` re-pointed at `host`."""
    out = json.loads(json.dumps(plan))  # deep copy, key order preserved
    platform = out.get('adapter')
    if platform not in cfg.PORTS:
        raise SystemExit(f'unknown adapter {platform!r}; cannot derive ports')
    old = out.get('target') or {}
    new = {}
    for key, port in cfg.PORTS[platform].items():
        if key in old or key == 'base_url':
            new[key] = f'http://{host}:{port}'
    # Preserve any extra target keys the plan had (fixture-only flags, etc.).
    for key, value in old.items():
        if key not in new:
            new[key] = value
    out['target'] = new
    return out


def differing_fields(a, b, path: str = '') -> list[str]:
    """Every JSON path where two documents differ."""
    out: list[str] = []
    if isinstance(a, dict) and isinstance(b, dict):
        for key in sorted(set(a) | set(b)):
            out += differing_fields(a.get(key), b.get(key), f'{path}.{key}' if path else key)
    elif isinstance(a, list) and isinstance(b, list):
        if len(a) != len(b):
            out.append(f'{path}[len]')
        else:
            for i, (x, y) in enumerate(zip(a, b)):
                out += differing_fields(x, y, f'{path}[{i}]')
    elif a != b:
        out.append(path or '<root>')
    return out


def rehost_target(target: dict, host: str) -> dict:
    """The same target URLs with only the host replaced (ports and paths kept).

    Used to check a resolved plan against the frozen one after a rebase: the
    resolved target must equal the frozen target *with its host swapped*, which
    proves the ports did not move even though the address did.
    """
    out = {}
    for key, value in (target or {}).items():
        if isinstance(value, str) and '://' in value:
            parts = urlsplit(value)
            netloc = host if parts.port is None else f'{host}:{parts.port}'
            out[key] = parts._replace(netloc=netloc).geturl()
        else:
            out[key] = value
    return out


def target_matches(resolved_target: dict, frozen_target: dict, host: str) -> bool:
    """True when the resolved target is the frozen one re-pointed at `host`."""
    return (resolved_target or {}) == rehost_target(frozen_target, host)


def workload_differences(a: dict, b: dict) -> list[str]:
    """Differences outside `target`: these must always be empty."""
    return [d for d in differing_fields(a, b) if not (d == 'target' or d.startswith('target.'))]


def overlay_plan(frozen_path: Path, out_path: Path, host: str) -> dict:
    """Write a target-only overlay of `frozen_path` and return its record.

    Refuses to write anything if the rewrite would touch a field outside
    `target`, so a bug here cannot silently change the experiment.
    """
    frozen = json.loads(Path(frozen_path).read_text())
    rebased = rewrite_target(frozen, host)
    outside = workload_differences(frozen, rebased)
    if outside:
        raise SystemExit('refusing overlay: rewrite would change the workload: '
                         + ', '.join(outside[:10]))
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(rebased, indent=2) + '\n')
    return {
        'kind': 'target-overlay-v1',
        'frozen_plan': str(frozen_path),
        'frozen_sha256': sha256_file(frozen_path),
        'overlay_plan': str(out_path),
        'overlay_sha256': sha256_file(out_path),
        'from_hosts': sorted(target_hosts(frozen)),
        'to_host': host,
        'changed_paths': differing_fields(frozen, rebased),
        'workload_differences': [],
        'note': ('Only the target block was re-pointed at the deployment host. '
                 'Every other plan field is byte-identical to the frozen plan.'),
    }


def verify_overlay(frozen_path: Path, overlay_path: Path, record: dict | None = None) -> list[str]:
    """Return a list of problems; empty means the overlay is honest."""
    problems = []
    frozen_path, overlay_path = Path(frozen_path), Path(overlay_path)
    if not frozen_path.is_file():
        return [f'frozen plan missing: {frozen_path}']
    if not overlay_path.is_file():
        return [f'overlay plan missing: {overlay_path}']
    if record is not None:
        if record.get('frozen_sha256') != sha256_file(frozen_path):
            problems.append('frozen plan changed since the overlay was recorded')
        if record.get('overlay_sha256') != sha256_file(overlay_path):
            problems.append('overlay plan changed since it was recorded')
    frozen = json.loads(frozen_path.read_text())
    overlay = json.loads(overlay_path.read_text())
    outside = workload_differences(frozen, overlay)
    if outside:
        problems.append('overlay changes the workload: ' + ', '.join(outside[:10]))
    if not (frozen.get('target') or {}):
        problems.append('frozen plan has no target block')
    return problems
